package executor

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
)

func newServerErrorOfBackend(name, method string, e error) error {
	return errors.NewCommonServerError(ResourceNameBackend, name, method, e.Error())
}

func (q *QingCloudClient) DeleteBackends(ids []*string) error {
	if len(ids) == 0 {
		klog.Warningln("No backends to delete, pls check the inputs")
		return nil
	}
	output, err := q.LBService.DeleteLoadBalancerBackends(&qcservice.DeleteLoadBalancerBackendsInput{
		LoadBalancerBackends: ids,
	})
	if *output.RetCode != 0 {
		return errors.NewCommonServerError(ResourceNameBackend, spew.Sdump(ids), "DeleteBackends", *output.Message)
	}
	return err
}

//need update lb
func (q *QingCloudClient) CreateBackends(backends []*apis.LoadBalancerBackend) ([]*string, error) {
	if len(backends) <= 0 {
		return nil, nil
	}

	input := &qcservice.AddLoadBalancerBackendsInput{
		Backends:             nil,
		LoadBalancerListener: backends[0].Spec.LoadBalancerListenerID,
	}
	for _, backend := range backends {
		input.Backends = append(input.Backends, &qcservice.LoadBalancerBackend{
			LoadBalancerBackendName: backend.Spec.LoadBalancerBackendName,
			Port:                    backend.Spec.Port,
			ResourceID:              backend.Spec.ResourceID,
		})
	}

	output, err := q.LBService.AddLoadBalancerBackends(input)
	if err != nil || *output.RetCode != 0 {
		return nil, fmt.Errorf("failed to create backends, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
	}

	return output.LoadBalancerBackends, nil
}

func convertLoadBalancerBackend(backends []*qcservice.LoadBalancerBackend) []*apis.LoadBalancerBackend {
	var result []*apis.LoadBalancerBackend

	for _, backend := range backends {
		result = append(result, &apis.LoadBalancerBackend{
			Spec: apis.LoadBalancerBackendSpec{
				LoadBalancerBackendName: backend.LoadBalancerBackendName,
				Port:                    backend.Port,
				ResourceID:              backend.ResourceID,
			},
			Status: apis.LoadBalancerBackendStatus{
				LoadBalancerBackendID: backend.LoadBalancerBackendID,
			},
		})
	}

	return result
}
