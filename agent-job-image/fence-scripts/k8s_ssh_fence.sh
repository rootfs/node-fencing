#!/bin/sh
echo Running ssh fence to $1
ssh -o ConnectTimeout=3 root@$1 'service kubelet restart'
exit $?

