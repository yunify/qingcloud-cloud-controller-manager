package e2eutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/yunify/qingcloud-sdk-go/config"
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
