package executor

import (
	"strings"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
)

func newServerErrorOfListener(name, method string, e error) error {
	return errors.NewCommonServerError(ResourceNameListener, name, method, e.Error())
}

func (q *qingCloudLoadBalanceExecutor) GetListenersOfLB(id, prefix string) ([]*qcservice.LoadBalancerListener, error) {
	resp, err := q.lbapi.DescribeLoadBalancerListeners(&qcservice.DescribeLoadBalancerListenersInput{
		LoadBalancer: &id,
		Verbose:      qcservice.Int(1),
		Limit:        qcservice.Int(pageLimt),
	})
	if err != nil {
		return nil, newServerErrorOfListener(id, "Wait Deletion Done", err)
	}
	if prefix == "" {
		return resp.LoadBalancerListenerSet, nil
	}
	loadBalancerListeners := []*qcservice.LoadBalancerListener{}
	for _, l := range resp.LoadBalancerListenerSet {
		if strings.Contains(*l.LoadBalancerListenerName, prefix) {
			loadBalancerListeners = append(loadBalancerListeners, l)
		}
	}
	return loadBalancerListeners, nil
}

func (q *qingCloudLoadBalanceExecutor) DeleteListener(lsnid string) error {
	output, err := q.lbapi.DeleteLoadBalancerListeners(&qcservice.DeleteLoadBalancerListenersInput{
		LoadBalancerListeners: []*string{&lsnid},
	})
	if err != nil {
		return newServerErrorOfListener(lsnid, "DeleteListener", err)
	}
	if *output.RetCode != 0 {
		return errors.NewCommonServerError(ResourceNameListener, lsnid, "DeleteListener", *output.Message)
	}
	return nil
}

func (q *qingCloudLoadBalanceExecutor) CreateListener(input *qcservice.AddLoadBalancerListenersInput) (*qcservice.LoadBalancerListener, error) {
	output, err := q.lbapi.AddLoadBalancerListeners(input)
	if err != nil {
		return nil, newServerErrorOfListener(*input.Listeners[0].LoadBalancerListenerName, "CreateListener", err)
	}
	if *output.RetCode != 0 {
		return nil, errors.NewCommonServerError(ResourceNameListener, *input.LoadBalancer, "CreateListener", *output.Message)
	}
	return q.GetListenerByID(*output.LoadBalancerListeners[0])
}

func (q *qingCloudLoadBalanceExecutor) GetListenerByID(id string) (*qcservice.LoadBalancerListener, error) {
	output, err := q.lbapi.DescribeLoadBalancerListeners(&qcservice.DescribeLoadBalancerListenersInput{
		LoadBalancerListeners: []*string{&id},
	})
	if err != nil {
		return nil, newServerErrorOfListener(id, "GetListenerByID", err)
	}
	if len(output.LoadBalancerListenerSet) == 0 {
		return nil, errors.NewResourceNotFoundError(ResourceNameListener, id)
	}
	return output.LoadBalancerListenerSet[0], nil
}

func (q *qingCloudLoadBalanceExecutor) GetListenerByName(id, name string) (*qcservice.LoadBalancerListener, error) {
	output, err := q.lbapi.DescribeLoadBalancerListeners(&qcservice.DescribeLoadBalancerListenersInput{
		LoadBalancer: &id,
	})
	if err != nil {
		return nil, newServerErrorOfListener(name, "GetListenerByName", err)
	}
	for _, list := range output.LoadBalancerListenerSet {
		if *list.LoadBalancerListenerName == name {
			return list, nil
		}
	}
	return nil, errors.NewResourceNotFoundError(ResourceNameListener, name)
}

func (q *qingCloudLoadBalanceExecutor) ModifyListener(id, balanceMode string) error {
	output, err := q.lbapi.ModifyLoadBalancerListenerAttributes(&qcservice.ModifyLoadBalancerListenerAttributesInput{
		LoadBalancerListener: &id,
		BalanceMode:          &balanceMode,
	})
	if err != nil {
		return newServerErrorOfListener(id, "ModifyListener", err)
	}
	if *output.RetCode != 0 {
		return errors.NewCommonServerError(ResourceNameListener, id, "ModifyListener", *output.Message)
	}
	return nil
}
