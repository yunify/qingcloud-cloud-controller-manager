package loadbalance_test

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"strings"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/loadbalance"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var _ = Describe("Loadbalance", func() {
	It("Should follow the topology", func() {
		testService := &corev1.Service{}
		service := `
kind: Service
apiVersion: v1
metadata:
  name:  mylbapp
  namespace: default
  annotations:
    service.beta.kubernetes.io/qingcloud-load-balancer-eip-ids: "eip-vmldumvv"
    service.beta.kubernetes.io/qingcloud-load-balancer-type: "0"
spec:
  selector:
    app:  mylbapp
  type:  LoadBalancer 
  ports:
  - name:  http
    port:  8088
    targetPort:  80`
		reader := strings.NewReader(service)
		err := yaml.NewYAMLOrJSONDecoder(reader, 10).Decode(testService)
		Expect(err).ShouldNot(HaveOccurred(), "Cannot unmarshal yamls")
		testService.SetUID(types.UID("11111-2222-3333"))
		lb := loadbalance.NewLoadBalancer(&loadbalance.NewLoadBalancerOption{
			K8sService:  testService,
			ClusterName: "Test",
			Context:     context.TODO(),
		})
		Expect(lb).ShouldNot(BeNil())
		Expect(lb.Name).To(Equal("k8s_lb_Test_mylbapp_11111"))
		Expect(lb.LoadListeners()).ShouldNot(HaveOccurred())
		Expect(lb.GetListeners()).To(HaveLen(1))
		listener := lb.GetListeners()[0]
		Expect(listener.Name).To(Equal("listener_default_mylbapp_8088"))
		Expect(listener.ListenerPort).To(Equal(8088))
	})
})
