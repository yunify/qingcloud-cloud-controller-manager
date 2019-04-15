package e2e_test

import (
	"fmt"
	"log"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/e2eutil"
	"github.com/yunify/qingcloud-sdk-go/service"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("E2e", func() {
	It("Should work as expected when using sample yamls", func() {
		//apply service
		service1Path := workspace + "/test/samples/service.yaml"
		serviceName := "mylbapp"
		testip := "139.198.121.161"
		Expect(e2eutil.KubectlApply(service1Path)).ShouldNot(HaveOccurred())
		defer func() {
			Expect(e2eutil.KubectlDelete(service1Path)).ShouldNot(HaveOccurred())
			//make sure lb is deleted
			lbService, _ := qcService.LoadBalancer("ap2a")
			Eventually(func() error {
				unaccept1 := "pending"
				unaccept2 := "active"
				key := "k8s_mylbapp"
				input := &service.DescribeLoadBalancersInput{
					Status:     []*string{&unaccept1, &unaccept2},
					SearchWord: &key,
				}
				output, err := lbService.DescribeLoadBalancers(input)
				if err != nil {
					return err
				}
				if len(output.LoadBalancerSet) == 0 {
					return nil
				}
				log.Printf("id:%s, name:%s, status:%s", *output.LoadBalancerSet[0].LoadBalancerID, *output.LoadBalancerSet[0].LoadBalancerName, *output.LoadBalancerSet[0].Status)
				return fmt.Errorf("LB has not been deleted")
			}, time.Minute*2, time.Second*10).Should(Succeed())
		}()
		Eventually(func() error {
			service, err := k8sclient.CoreV1().Services("default").Get(serviceName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if len(service.Status.LoadBalancer.Ingress) > 0 && service.Status.LoadBalancer.Ingress[0].IP == testip {
				return nil
			}
			return fmt.Errorf("Still got no ip")
		}, 3*time.Minute, 20*time.Second).Should(Succeed())
		log.Println("Successfully assign a ip")
		url := fmt.Sprintf("http://%s:%d", testip, 8088)
		Eventually(func() int {
			resp, err := http.Get(url)
			if err != nil {
				log.Println("Error in sending request,err: " + err.Error())
				return -1
			}
			return resp.StatusCode
		}, time.Second*20, time.Minute*5).Should(Equal(http.StatusOK))
		log.Println("Successfully get a 200 response")
	})
})
