# antek-async

K3s desktop client for Linux and Windows. Reads configuration from kubeconfig and executes Kubernetes API calls the way kubectl does, no Agent inside the cluster, no custom-resource definitions. This project was started to make it easier to monitor Kubernetes clusters, without needing to SSH into the server or run kubectl locally.

Besides monitoring, the Manifest YAML menu applies one YAML manifest to the active cluster with server-side apply, the same way `kubectl apply` does. The Namespaces menu can also delete a namespace, but only after the word `delete` and the namespace name are typed back into the confirmation dialog.

The Networking menu carries a Gateway API submenu with GatewayClass, Gateway, HTTPRoute and GRPCRoute. Those four are custom resources rather than built-in kinds, so they list when the Gateway API CRDs are installed and each page says so plainly when they are not.

The Pods and Nodes tables show CPU and memory in a single column, read from metrics-server every five seconds while that menu is open. The values are formatted the way `kubectl top` prints them, and a cluster without the metrics API shows `-` there while the rest of the list works as usual.

![antek-async](static/quick-view.png)
