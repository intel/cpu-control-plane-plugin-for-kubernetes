#!/bin/bash
## Modified from Ethan Davidson
## https://stackoverflow.com/questions/71584005/
## how-to-run-multi-fuzz-test-cases-wirtten-in-one-source-file-with-go1-18

# clean all subprocesses on ctl-c
trap "trap - SIGTERM && kill -- -$$ || true" SIGINT SIGTERM

set -e

fuzzTime="${1:-1}"m  # read from argument list or fallback to default - 1 minute

files=$(grep -r --include='**_test.go' --files-with-matches 'func Fuzz' .)

logsdir="$(dirname "$0")/../fuzzlogs"
mkdir -p "${logsdir}"

cat <<EOF
Starting fuzzing tests.
    One test timeout: $fuzzTime
    Files: $files
    Logs dir: $logsdir
EOF

go clean --cache

for file in ${files}
do
    funcs="$(grep -oP 'func \K(Fuzz\w*)' "$file")"
    for func in ${funcs}
    do
        {
            echo "Fuzzing $func in $file"
            parentDir="$(dirname "$file")"
            go test "$parentDir" -run="$func" -fuzz="$func" -fuzztime="${fuzzTime}" -v -parallel 4 ./... \
            | tee "${logsdir}"/"$func."log
        } &
    done
done

for job in `jobs -p`
do
    echo "Waiting for PID $job to finish"
    wait $job || true
done
