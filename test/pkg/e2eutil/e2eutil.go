package e2eutil

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/yunify/qingcloud-sdk-go/config"
	"github.com/yunify/qingcloud-sdk-go/service"
	qc "github.com/yunify/qingcloud-sdk-go/service"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

func WaitForController(c *kubernetes.Clientset, namespace, name string, retryInterval, timeout time.Duration) error {
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		controller, err := c.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			fmt.Println("Cannot find controller")
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if controller.Status.ReadyReplicas == 1 {
			return true, nil
		}
		return false, nil
	})
	return err
}

func KubectlApply(filename string) error {
	cmd := exec.Command("kubectl", "apply", "-f", filename)
	str, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("kubectl apply failed, error :%s\n", str)
	}
	return err
}

func KubectlDelete(filename string) error {
	ctx, cancle := context.WithTimeout(context.Background(), time.Second*20)
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "-f", filename)
	defer cancle()
	return cmd.Run()
}

func GetQingcloudService() (*qc.QingCloudService, error) {
	accessKey := os.Getenv("ACCESS_KEY_ID")
	secret := os.Getenv("SECRET_ACCESS_KEY")
	configuration, _ := config.New(accessKey, secret)
	configuration.Zone = "ap2a"
	return qc.Init(configuration)
}

func ServiceHasEIP(c *kubernetes.Clientset, name, namespace, ip string) error {
	service, err := c.CoreV1().Services(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if len(service.Status.LoadBalancer.Ingress) > 0 && service.Status.LoadBalancer.Ingress[0].IP == ip {
		return nil
	}
	return fmt.Errorf("Still got no ip")
}

func GerServiceResponse(ip string, port int) int {
	url := fmt.Sprintf("http://%s:%d", ip, port)
	resp, err := http.Get(url)
	if err != nil {
		log.Println("Error in sending request,err: " + err.Error())
		return -1
	}
	return resp.StatusCode
}

func WaitForLoadBalancerDeleted(lbService *service.LoadBalancerService) error {
	unaccept1 := "pending"
	unaccept2 := "active"
	key := "k8s_lb_kubernetes"
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
}
