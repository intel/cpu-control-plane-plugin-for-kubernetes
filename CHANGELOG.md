# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
## 0.1.2[01.06.2023]
### Version Update
- update golang version to 1.20.4
## 0.1.1[24.01.2023]
### Bugfixes
- numa-namespace policies handles correclty more namespaces than numa zones by grouping them 
- report error when runtime agent configuration does not match the system runtime
- correct representation of memory resources
- correct cgroup v1 path validation 

## 0.1 [02.12.2022]
### Added
- Add support for cgroups v2
- Add support for cgroupfs cgroup driver
- Add exclusive variant of numa-namespace policy. It makes guaranteed pods an exclusive access to cpus.
- Add numa-namespace policy. It enables guaranteed , burstable and best-effort pod insolation in a numa zone based on namespace.
- Add numa policy. It enables single numa policy.
- Add default policy. It enables std. static cpu managment mode without topology-management.
- Add support for Kind cluster for running integration tests
- Use klog logging
- Add support for containerd
