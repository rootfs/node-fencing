#!/usr/bin/python

import googleapiclient.discovery
import sys
compute = googleapiclient.discovery.build('compute', 'v1')

i = compute.instances()

instance_name = sys.argv[1]

i.reset(project="kube-cluster-fence-poc", zone="us-east1-b", instance=instance_name).execute()
