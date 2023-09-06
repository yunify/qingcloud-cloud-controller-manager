package executor

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog/v2"

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
	input := &qcservice.DeleteLoadBalancerBackendsInput{
		LoadBalancerBackends: ids,
	}
	output, err := q.LBService.DeleteLoadBalancerBackends(input)
	if err != nil || *output.RetCode != 0 {
		klog.V(4).Infof("failed to delete backends, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
		if err != nil {
			return fmt.Errorf("failed to delete backends,err=%v", err)
		}
		return fmt.Errorf("failed to delete backends, code=%d, msg=%s", *output.RetCode, *output.Message)
	}
	return nil
}

// need update lb
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
		klog.V(4).Infof("failed to create backends, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
		if err != nil {
			return nil, fmt.Errorf("failed to create backends,err=%v", err)
		}
		return nil, fmt.Errorf("failed to create backends, code=%d, msg=%s", *output.RetCode, *output.Message)
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
