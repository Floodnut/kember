# Deploy

This directory contains the alpha CRDs, RBAC, operator Deployment, and sample
resources. Helm is not part of the current distribution.

The operator image must already be available to the target cluster. For a kind
cluster, load it before installation:

```bash
kind load docker-image --name kember-e2e kember-operator:e2e
KEMBER_OPERATOR_IMAGE=kember-operator:e2e ./deploy/install.sh
```

`install.sh` applies the namespace, CRDs, RBAC, and Deployment, then waits for
the operator rollout. It uses the current kubectl context.

After installation, apply a sample WorkerPool and TaskRun from
`deploy/samples/`. `kember-platform-admin` is intended to manage WorkerPools;
`kember-taskrun-developer` is intended to create TaskRuns and patch
`spec.cancel`.
