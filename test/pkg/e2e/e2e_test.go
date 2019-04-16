package e2e_test

import (
	"log"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/e2eutil"
)

var testip = "139.198.121.161"
var _ = Describe("E2e", func() {
	It("Should work as expected in ReUse Mode", func() {
		servicePath := workspace + "/test/test_cases/reuse/case.yaml"
		service1Name := "reuse-eip1"
		service2Name := "reuse-eip2"
		Expect(e2eutil.KubectlApply(servicePath)).ShouldNot(HaveOccurred())

		defer func() {
			Expect(e2eutil.KubectlDelete(servicePath)).ShouldNot(HaveOccurred())
			//make sure lb is deleted
			lbService, _ := qcService.LoadBalancer("ap2a")
			Eventually(func() error { return e2eutil.WaitForLoadBalancerDeleted(lbService) }, time.Minute*2, time.Second*15).Should(Succeed())
		}()

		Eventually(func() error {
			return e2eutil.ServiceHasEIP(k8sclient, service1Name, "default", testip)
		}, 3*time.Minute, 20*time.Second).Should(Succeed())
		Eventually(func() error {
			return e2eutil.ServiceHasEIP(k8sclient, service2Name, "default", testip)
		}, 2*time.Minute, 20*time.Second).Should(Succeed())

		log.Println("Successfully assign a ip")

		Eventually(func() int { return e2eutil.GerServiceResponse(testip, 8089) }, time.Second*20, time.Minute*5).Should(Equal(http.StatusOK))
		Eventually(func() int { return e2eutil.GerServiceResponse(testip, 8090) }, time.Second*20, time.Minute*5).Should(Equal(http.StatusOK))
		log.Println("Successfully get a 200 response")
	})

	It("Should work as expected when using sample yamls", func() {
		//apply service
		service1Path := workspace + "/test/test_cases/service.yaml"
		serviceName := "mylbapp"
		Expect(e2eutil.KubectlApply(service1Path)).ShouldNot(HaveOccurred())
		defer func() {
			Expect(e2eutil.KubectlDelete(service1Path)).ShouldNot(HaveOccurred())
			//make sure lb is deleted
			lbService, _ := qcService.LoadBalancer("ap2a")
			Eventually(func() error { return e2eutil.WaitForLoadBalancerDeleted(lbService) }, time.Minute*1, time.Second*15).Should(Succeed())
		}()
		Eventually(func() error {
			return e2eutil.ServiceHasEIP(k8sclient, serviceName, "default", testip)
		}, 3*time.Minute, 20*time.Second).Should(Succeed())
		log.Println("Successfully assign a ip")
		Eventually(func() int { return e2eutil.GerServiceResponse(testip, 8088) }, time.Second*20, time.Minute*5).Should(Equal(http.StatusOK))
		log.Println("Successfully get a 200 response")
	})
})
