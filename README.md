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

# disclaimer
1. Flag **ARGOCD_APPLICATION_NAMESPACES** was introduced as part of https://argo-cd.readthedocs.io/en/latest/operator-manual/app-any-namespace/#change-workload-startup-parameters feature, so in this repo the same flag has been reused.