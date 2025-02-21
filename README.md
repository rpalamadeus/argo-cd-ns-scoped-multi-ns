# argo-cd-ns-scoped-multi-ns
This repo contains changes related to running argocd in namespaced mode along with managing multiple namespace each having their own argoproj resources

# features
1. Flag **ARGOCD_NOTIFICATION_CONTROLLER_SELF_SERVICE_NOTIFICATION_ENABLED** introduced, so that a single notifications controller can serve multiple argocd instances, when **ARGOCD_APPLICATION_NAMESPACES** contains ns-a,ns-b, etc.. where ns-a and ns-b are namespaces of argocd instances. In Deployment yaml env properties needs to be added as below -
```
            - name: ARGOCD_NAMESPACED_MODE_MULTI_NAMESPACE_ENABLED
              valueFrom:
                configMapKeyRef:
                  key: namespacedmode.multinamespace.enabled
                  name: argocd-cmd-params-cm
                  optional: true
            - name: ARGOCD_APPLICATION_NAMESPACES
              valueFrom:
                configMapKeyRef:
                  key: application.namespaces
                  name: argocd-cmd-params-cm
                  optional: true

```
2. Flag **ARGOCD_NOTIFICATION_CONTROLLER_SELF_SERVICE_NOTIFICATION_ENABLED** introduced, so that a single appset controller can serve multiple argocd instances, when **ARGOCD_APPLICATION_NAMESPACES** contains ns-a,ns-b, etc.. where ns-a and ns-b are namespaces of argocd instances In Deployment yaml env properties needs to be added as below -
```
            - name: ARGOCD_NAMESPACED_MODE_MULTI_NAMESPACE_ENABLED
              valueFrom:
                configMapKeyRef:
                  key: namespacedmode.multinamespace.enabled
                  name: argocd-cmd-params-cm
                  optional: true
            - name: ARGOCD_APPLICATION_NAMESPACES
              valueFrom:
                configMapKeyRef:
                  key: application.namespaces
                  name: argocd-cmd-params-cm
                  optional: true
```
3. Benchmarking a test k8s informer client for pods across all namespaces if used with compressor and external cache

Command executed format
```
go run . --namespace= --compress=true --external-cache=true
go run . --namespace= --compress=false --external-cache=false
go run . --namespace= --compress=false --external-cache=true 
go run . --namespace= --compress=true --external-cache=false
```

| **--compress** | **--external-cache** | **Alloc** | **Sys** | **HeapAlloc** | **HeapSys** |
|---------------|---------------|---------------|---------------|---------------|-------------|
| true  | true  | 74 MB  | 234 MB | 74 MB   | 223 MB  |
| false | false | 119 MB | 231 MB | 119 MB  | 219 MB  |
| false | true  | 75 MB  | 231 MB | 75 MB   | 219 MB  |
| true  | false | 130 MB | 230 MB | 130 MB  | 219 MB  |


# disclaimer
1. Flag **ARGOCD_APPLICATION_NAMESPACES** was introduced as part of https://argo-cd.readthedocs.io/en/latest/operator-manual/app-any-namespace/#change-workload-startup-parameters feature, so in this repo the same flag has been reused.


