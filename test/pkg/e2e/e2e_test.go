package e2e_test

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/loadbalance"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/e2eutil"
	"github.com/yunify/qingcloud-sdk-go/service"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var testip = "139.198.121.161"
var ipchange = "139.198.121.98"
var _ = Describe("E2e", func() {
	It("Should work as expected in ReUse Mode", func() {
		servicePath := workspace + "/test/test_cases/reuse/case.yaml"
		service1Name := "reuse-eip1"
		service2Name := "reuse-eip2"
		Expect(e2eutil.KubectlApply(servicePath)).ShouldNot(HaveOccurred())
		defer func() {
			Expect(e2eutil.KubectlDelete(servicePath)).ShouldNot(HaveOccurred())
			time.Sleep(1 * time.Minute)
			//make sure lb is deleted
			lbService, _ := qcService.LoadBalancer("ap2a")
			Eventually(func() error { return e2eutil.WaitForLoadBalancerDeleted(lbService) }, time.Minute*2, time.Second*10).Should(Succeed())
		}()
		log.Println("Just wait 2 minutes before tests because following procedure is so so so slow ")
		time.Sleep(2 * time.Minute)
		log.Println("Wake up, we can test now")
		Eventually(func() error {
			return e2eutil.ServiceHasEIP(k8sclient, service1Name, "default", testip)
		}, 2*time.Minute, 20*time.Second).Should(Succeed())
		Eventually(func() error {
			return e2eutil.ServiceHasEIP(k8sclient, service2Name, "default", testip)
		}, 1*time.Minute, 5*time.Second).Should(Succeed())

		log.Println("Successfully assign a ip")

		Eventually(func() int { return e2eutil.GerServiceResponse(testip, 8089) }, time.Second*20, time.Second*5).Should(Equal(http.StatusOK))
		Eventually(func() int { return e2eutil.GerServiceResponse(testip, 8090) }, time.Second*20, time.Second*5).Should(Equal(http.StatusOK))
		log.Println("Successfully get a 200 response")
	})

	It("Should try to use available ips when user does not specify the ip", func() {
		service1Path := workspace + "/test/test_cases/eip/use_available.yaml"
		serviceName := "mylbapp"
		Expect(e2eutil.KubectlApply(service1Path)).ShouldNot(HaveOccurred())
		defer func() {
			log.Println("Deleting test svc")
			Expect(e2eutil.KubectlDelete(service1Path)).ShouldNot(HaveOccurred())
			//make sure lb is deleted
			lbService, _ := qcService.LoadBalancer("ap2a")
			time.Sleep(time.Second * 30)
			Eventually(func() error { return e2eutil.WaitForLoadBalancerDeleted(lbService) }, time.Minute*1, time.Second*10).Should(Succeed())
		}()
		log.Println("Just wait 2 minutes before tests because following procedure is so so so slow ")
		time.Sleep(2 * time.Minute)
		log.Println("Wake up, we can test now")
		Eventually(func() error {
			return e2eutil.ServiceHasEIP(k8sclient, serviceName, "default", "")
		}, 3*time.Minute, 20*time.Second).Should(Succeed())
		log.Println("Successfully assign a ip")
	})

	It("Should work as expected when using sample yamls", func() {
		//apply service
		service1Path := workspace + "/test/test_cases/service.yaml"
		serviceName := "mylbapp"
		Expect(e2eutil.KubectlApply(service1Path)).ShouldNot(HaveOccurred())
		defer func() {
			log.Println("Deleting test svc")
			Expect(e2eutil.KubectlDelete(service1Path)).ShouldNot(HaveOccurred())
			//make sure lb is deleted
			lbService, _ := qcService.LoadBalancer("ap2a")
			time.Sleep(time.Second * 30)
			Eventually(func() error { return e2eutil.WaitForLoadBalancerDeleted(lbService) }, time.Minute*1, time.Second*10).Should(Succeed())
		}()
		log.Println("Just wait 2 minutes before tests because following procedure is so so so slow ")
		time.Sleep(2 * time.Minute)
		log.Println("Wake up, we can test now")
		Eventually(func() error {
			return e2eutil.ServiceHasEIP(k8sclient, serviceName, "default", testip)
		}, 3*time.Minute, 20*time.Second).Should(Succeed())
		log.Println("Successfully assign a ip")
		Eventually(func() int { return e2eutil.GerServiceResponse(testip, 8088) }, time.Second*20, time.Second*5).Should(Equal(http.StatusOK))
		log.Println("Successfully get a 200 response")

		//update size
		svc, err := k8sclient.CoreV1().Services("default").Get(serviceName, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		expectedType := 2
		svc.Annotations[loadbalance.ServiceAnnotationLoadBalancerType] = strconv.Itoa(expectedType)
		svc.Annotations[loadbalance.ServiceAnnotationLoadBalancerEipIds] = "eip-e5fxdepa"
		svc, err = k8sclient.CoreV1().Services("default").Update(svc)
		Expect(err).ShouldNot(HaveOccurred(), "Failed to update svc")
		log.Println("Just wait 3 minutes before tests because following procedure is so so so slow ")
		time.Sleep(3 * time.Minute)
		log.Println("Wake up, we can test now")
		lbService, _ := qcService.LoadBalancer("ap2a")
		name := loadbalance.GetLoadBalancerName("kubernetes", svc)
		Eventually(func() error {
			input := &service.DescribeLoadBalancersInput{
				Status:     []*string{service.String("active")},
				SearchWord: &name,
			}
			output, err := lbService.DescribeLoadBalancers(input)
			if err != nil {
				return err
			}
			if len(output.LoadBalancerSet) == 1 && *output.LoadBalancerSet[0].LoadBalancerType == expectedType {
				if len(output.LoadBalancerSet[0].Cluster) == 1 && *output.LoadBalancerSet[0].Cluster[0].EIPAddr == ipchange {
					return nil
				}
			}
			return fmt.Errorf("Lb type is not changed or ip not change")
		}, 3*time.Minute, 20*time.Second).Should(Succeed())
		Eventually(func() int { return e2eutil.GerServiceResponse(ipchange, 8088) }, time.Second*20, time.Second*2).Should(Equal(http.StatusOK))
		log.Println("Successfully get a 200 response after resizing")
	})
})
