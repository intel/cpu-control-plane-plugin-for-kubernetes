// Package agent implements ctlplane agent which observes k8s for pod lifecycle events
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"resourcemanagement.controlplane/pkg/ctlplaneapi"
)

const (
	defaultTimeout          = 5 * time.Second
	maxUnsuccesfullAttempts = 3
)

var ErrCannotSync = errors.New("cannot sync with k8s")

// Agent observes k8s for pod lifecycle events.
type Agent struct {
	ctlPlaneClient                     ctlplaneapi.ControlPlaneClient
	mu                                 sync.Mutex
	addedPods                          map[types.UID]bool
	namespacePrefix                    string
	ctx                                context.Context
	callTimeout                        time.Duration
	logger                             logr.Logger
	numConsecutiveUnsuccessfulAttempts uint
}

// NewAgent returns new agent with fields properly initialized.
func NewAgent(context context.Context, ctlPlaneClient ctlplaneapi.ControlPlaneClient, namespacePrefix string) *Agent {
	logger, err := logr.FromContext(context)
	if err != nil {
		klog.Fatal("no logger provided")
	}
	return &Agent{
		ctlPlaneClient:  ctlPlaneClient,
		namespacePrefix: namespacePrefix,
		addedPods:       make(map[types.UID]bool),
		ctx:             context,
		callTimeout:     defaultTimeout,
		logger:          logger.WithName("agent"),
	}
}

func (a *Agent) context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(a.ctx, a.callTimeout)
}

// Run runs agent loop in a goroutine.
func (a *Agent) Run(clusterClient kubernetes.Interface, nodeName string) error {
	factory := informers.NewSharedInformerFactoryWithOptions(clusterClient, 0, informers.WithNamespace(""),
		informers.WithTweakListOptions(func(o *metav1.ListOptions) {
			o.LabelSelector = "app!=ctlplane-daemonset"
			o.FieldSelector = fmt.Sprintf("spec.nodeName=%s", nodeName)
		}),
	)
	podInformer := factory.Core().V1().Pods()
	informer := podInformer.Informer()

	defer runtime.HandleCrash()

	go factory.Start(a.ctx.Done())

	a.logger.Info("syncing cache")
	synced := cache.WaitForNamedCacheSync("ctlplane-agent:"+nodeName, a.ctx.Done(), informer.HasSynced)
	if !synced {
		a.logger.Error(ErrCannotSync, "could not sync k8s state")
		return ErrCannotSync
	}

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: a.update,
		DeleteFunc: a.delete,
	})
	a.logger.Info("agent started")
	return nil
}

// update is invoked whenever pod status changes. We use it also to send CreatePodRequest, because the
// update reports all changes in pod's containers, and we shall wait for all containers to be up and running
// before sending the request.
func (a *Agent) update(_ interface{}, newobj interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()

	p, ok := newobj.(*corev1.Pod)
	logger := a.logger.WithName("update")

	if !ok {
		logger.Info("obj passed is not a pod")
		return
	}

	logger = logger.WithValues("PID", p.UID)

	if !strings.HasPrefix(p.Namespace, a.namespacePrefix) {
		logger.V(2).Info("pod namespace does not contain prefix", "namespace", p.Namespace, "prefix", a.namespacePrefix)
		return
	}

	if p.DeletionTimestamp != nil {
		logger.Info("pod has deletion timestamp, ignoring")
		return
	}

	allContainersReady := true
	for _, c := range p.Status.ContainerStatuses {
		if c.ContainerID == "" || !c.Ready {
			allContainersReady = false
			break
		}
	}
	logger.V(2).Info("received pod update", "allContainersReady", allContainersReady)

	if !allContainersReady || len(p.Status.ContainerStatuses) != len(p.Spec.Containers) {
		return
	}

	var (
		reply *ctlplaneapi.PodAllocationReply
		err   error
	)
	if a.addedPods[p.UID] {
		in, reqErr := GetUpdatePodRequest(p)
		if reqErr != nil {
			err = reqErr
		} else {
			logger.Info("sending update pod req")
			ctx, cancel := a.context()
			defer cancel()
			reply, err = a.ctlPlaneClient.UpdatePod(ctx, in)
		}
	} else {
		in, reqErr := GetCreatePodRequest(p)
		if reqErr != nil {
			err = reqErr
		} else {
			logger.Info("sending add pod req")
			ctx, cancel := a.context()
			defer cancel()
			reply, err = a.ctlPlaneClient.CreatePod(ctx, in)
			a.addedPods[p.UID] = true
		}
	}

	if err != nil {
		logger.Error(err, "allocation error")
		a.unsuccessfulAttempt()
	} else {
		logger.Info("allocation done", "reply", reply)
		a.successfulAttempt()
	}
}

// delete is invoked after pod has been deleted.
func (a *Agent) delete(obj interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()

	logger := a.logger.WithName("delete")

	p, ok := obj.(*corev1.Pod)

	if !ok {
		logger.Info("obj passed is not a pod")
		return
	}

	logger = logger.WithValues("PID", p.UID)

	if !strings.HasPrefix(p.Namespace, a.namespacePrefix) {
		logger.V(2).Info("pod namespace does not contain prefix", "namespace", p.Namespace, "prefix", a.namespacePrefix)
		return
	}

	logger.Info("deleting pod")
	in := GetDeletePodRequest(p)
	ctx, cancel := a.context()
	defer cancel()
	reply, err := a.ctlPlaneClient.DeletePod(ctx, in)
	delete(a.addedPods, p.UID)

	if err != nil {
		logger.Error(err, "deletion failed")
		a.unsuccessfulAttempt()
	} else {
		logger.Info("deletion done", "reply", reply)
		a.successfulAttempt()
	}
}

func (a *Agent) successfulAttempt() {
	a.numConsecutiveUnsuccessfulAttempts = 0
}

func (a *Agent) unsuccessfulAttempt() {
	a.numConsecutiveUnsuccessfulAttempts += 1
	if a.numConsecutiveUnsuccessfulAttempts >= maxUnsuccesfullAttempts {
		klog.Fatal("Exceeded maximum number of unsuccessful attempts")
	}
}
