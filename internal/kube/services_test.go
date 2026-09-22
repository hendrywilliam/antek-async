package kube

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func TestServiceClusterIP(t *testing.T) {
	tests := []struct {
		name    string
		cluster []string
		want    string
	}{
		{
			name:    "ordinary service",
			cluster: []string{"10.43.0.10"},
			want:    "10.43.0.10",
		},
		{
			name:    "headless service",
			cluster: []string{"None"},
			want:    "None",
		},
		{
			name: "external name service",
			want: "<none>",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &corev1.Service{Spec: corev1.ServiceSpec{ClusterIPs: test.cluster}}
			if got := serviceClusterIP(service); got != test.want {
				t.Errorf("serviceClusterIP() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestServiceExternalIP(t *testing.T) {
	tests := []struct {
		name   string
		spec   corev1.ServiceSpec
		status corev1.ServiceStatus
		want   string
	}{
		{
			name: "cluster ip service exposes nothing",
			spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP},
			want: "<none>",
		},
		{
			name: "node port service exposes nothing on its own",
			spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort},
			want: "<none>",
		},
		{
			name: "explicit external ip is shown",
			spec: corev1.ServiceSpec{
				Type:        corev1.ServiceTypeClusterIP,
				ExternalIPs: []string{"203.0.113.7"},
			},
			want: "203.0.113.7",
		},
		{
			name: "load balancer without an address is pending",
			spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
			want: "<pending>",
		},
		{
			name: "load balancer address is taken from the ingress",
			spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
			status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{{IP: "198.51.100.4"}},
				},
			},
			want: "198.51.100.4",
		},
		{
			name: "ingress hostnames are used when there is no ip",
			spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
			status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{{Hostname: "lb.example.com"}},
				},
			},
			want: "lb.example.com",
		},
		{
			name: "ingresses are sorted and deduplicated",
			spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
			status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{IP: "198.51.100.9"},
						{IP: "198.51.100.4"},
						{IP: "198.51.100.9"},
					},
				},
			},
			want: "198.51.100.4,198.51.100.9",
		},
		{
			name: "load balancer address comes before explicit external ips",
			spec: corev1.ServiceSpec{
				Type:        corev1.ServiceTypeLoadBalancer,
				ExternalIPs: []string{"203.0.113.7"},
			},
			status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{{IP: "198.51.100.4"}},
				},
			},
			want: "198.51.100.4,203.0.113.7",
		},
		{
			name: "external name service shows its target",
			spec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "db.example.com",
			},
			want: "db.example.com",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &corev1.Service{Spec: test.spec, Status: test.status}
			if got := serviceExternalIP(service); got != test.want {
				t.Errorf("serviceExternalIP() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestServicePorts(t *testing.T) {
	tests := []struct {
		name  string
		ports []corev1.ServicePort
		want  string
	}{
		{
			name: "no ports",
			want: "<none>",
		},
		{
			name: "single port",
			ports: []corev1.ServicePort{
				{Port: 80, Protocol: corev1.ProtocolTCP},
			},
			want: "80/TCP",
		},
		{
			name: "node port is shown next to the port",
			ports: []corev1.ServicePort{
				{Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP},
			},
			want: "80:30080/TCP",
		},
		{
			name: "several ports are joined",
			ports: []corev1.ServicePort{
				{Port: 80, Protocol: corev1.ProtocolTCP},
				{Port: 53, NodePort: 3053, Protocol: corev1.ProtocolUDP},
			},
			want: "80/TCP,53:3053/UDP",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := servicePorts(test.ports); got != test.want {
				t.Errorf("servicePorts() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestToServiceInfo(t *testing.T) {
	now := time.Now()

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "shop",
			Name:              "web",
			CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Hour)),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeNodePort,
			ClusterIP: "10.43.0.20",
			ClusterIPs: []string{
				"10.43.0.20",
			},
			Ports: []corev1.ServicePort{
				{Port: 443, NodePort: 30443, Protocol: corev1.ProtocolTCP},
			},
		},
	}

	got := toServiceInfo(service, now)

	want := ServiceInfo{
		Namespace:  "shop",
		Name:       "web",
		Type:       "NodePort",
		ClusterIP:  "10.43.0.20",
		ExternalIP: "<none>",
		Ports:      "443:30443/TCP",
		Age:        "3h",
	}
	if got != want {
		t.Errorf("toServiceInfo() = %+v, want %+v", got, want)
	}
}

func TestServicesFromStoreSortsByNamespaceAndName(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	entries := []*corev1.Service{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "web"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "data", Name: "cache"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"}},
	}
	for _, service := range entries {
		if err := store.Add(service); err != nil {
			t.Fatalf("add %s/%s: %v", service.Namespace, service.Name, err)
		}
	}

	// A store can hold other types too, and those must be skipped.
	if err := store.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add pod: %v", err)
	}

	got := servicesFromStore(store)

	want := []string{"data/cache", "shop/api", "shop/web"}
	if len(got) != len(want) {
		t.Fatalf("got %d services, want %d", len(got), len(want))
	}
	for i, key := range want {
		gotKey := got[i].Namespace + "/" + got[i].Name
		if gotKey != key {
			t.Errorf("service %d = %q, want %q", i, gotKey, key)
		}
	}
}
