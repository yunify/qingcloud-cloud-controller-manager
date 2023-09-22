package executor

import (
	"fmt"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
)

func (q *QingCloudClient) GetListeners(id []*string) ([]*apis.LoadBalancerListener, error) {
	resp, err := q.LBService.DescribeLoadBalancerListeners(&qcservice.DescribeLoadBalancerListenersInput{
		LoadBalancerListeners: id,
		Verbose:               qcservice.Int(1),
		Limit:                 qcservice.Int(pageLimt),
	})
	if err != nil {
		return nil, err
	}

	return convertLoadBalancerListener(resp.LoadBalancerListenerSet), nil
}

func (q *QingCloudClient) DeleteListener(lsnid []*string) error {
	output, err := q.LBService.DeleteLoadBalancerListeners(&qcservice.DeleteLoadBalancerListenersInput{
		LoadBalancerListeners: lsnid,
	})
	if err != nil {
		return fmt.Errorf("failed to delete listener %v, err=%v", qcservice.StringValueSlice(lsnid), err)
	}
	if *output.RetCode != 0 {
		return fmt.Errorf("failed to delete listener %v, code=%d, msg=%s", qcservice.StringValueSlice(lsnid), *output.RetCode, *output.Message)
	}
	return nil
}

func convertLoadBalancerListener(inputs []*qcservice.LoadBalancerListener) []*apis.LoadBalancerListener {
	var result []*apis.LoadBalancerListener

	for _, input := range inputs {
		result = append(result, &apis.LoadBalancerListener{
			Spec: apis.LoadBalancerListenerSpec{
				BackendProtocol:          input.BackendProtocol,
				ListenerPort:             input.ListenerPort,
				ListenerProtocol:         input.ListenerProtocol,
				LoadBalancerListenerName: input.LoadBalancerListenerName,
				LoadBalancerListenerID:   input.LoadBalancerListenerID,
				LoadBalancerID:           input.LoadBalancerID,
				HealthyCheckMethod:       input.HealthyCheckMethod,
				HealthyCheckOption:       input.HealthyCheckOption,
				BalanceMode:              input.BalanceMode,
				ServerCertificateID:      input.ServerCertificateID,
				Timeout:                  input.Timeout,
			},
			Status: apis.LoadBalancerListenerStatus{
				LoadBalancerListenerID: input.LoadBalancerListenerID,
				LoadBalancerBackends:   convertLoadBalancerBackend(input.Backends),
			},
		})
	}

	return result
}

func convertFromLoadBalancerListener(inputs []*apis.LoadBalancerListener) []*qcservice.LoadBalancerListener {
	var result []*qcservice.LoadBalancerListener

	for _, input := range inputs {
		result = append(result, &qcservice.LoadBalancerListener{
			BackendProtocol:          input.Spec.BackendProtocol,
			ListenerPort:             input.Spec.ListenerPort,
			ListenerProtocol:         input.Spec.ListenerProtocol,
			LoadBalancerListenerName: input.Spec.LoadBalancerListenerName,
			HealthyCheckMethod:       input.Spec.HealthyCheckMethod,
			HealthyCheckOption:       input.Spec.HealthyCheckOption,
			BalanceMode:              input.Spec.BalanceMode,
			ServerCertificateID:      input.Spec.ServerCertificateID,
			Timeout:                  input.Spec.Timeout,
		})
	}

	return result
}

// need update lb
func (q *QingCloudClient) CreateListener(inputs []*apis.LoadBalancerListener) ([]*apis.LoadBalancerListener, error) {
	id := inputs[0].Spec.LoadBalancerID
	output, err := q.LBService.AddLoadBalancerListeners(&qcservice.AddLoadBalancerListenersInput{
		Listeners:    convertFromLoadBalancerListener(inputs),
		LoadBalancer: id,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create listener, err=%v", err)
	}
	if *output.RetCode != 0 {
		return nil, fmt.Errorf("failed to create listener, code=%d, msg=%s", *output.RetCode, *output.Message)
	}

	return q.GetListeners(output.LoadBalancerListeners)
}
