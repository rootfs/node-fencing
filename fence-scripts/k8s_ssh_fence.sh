#!/bin/sh
echo Running ssh fence to $1
ssh root@$1 'service kubelet restart'

