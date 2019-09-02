package executor

import (
	"strings"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog"
)

func newServerErrorOfBackend(name, method string, e error) error {
	return errors.NewCommonServerError(ResourceNameBackend, name, method, e.Error())
}

func (q *qingCloudLoadBalanceExecutor) DeleteBackends(ids ...string) error {
	if len(ids) == 0 {
		klog.Warningln("No backends to delete, pls check the inputs")
		return nil
	}
	output, err := q.lbapi.DeleteLoadBalancerBackends(&qcservice.DeleteLoadBalancerBackendsInput{
		LoadBalancerBackends: qcservice.StringSlice(ids),
	})
	if *output.RetCode != 0 {
		return errors.NewCommonServerError(ResourceNameBackend, strings.Join(ids, ";"), "DeleteBackends", *output.Message)
	}
	return err
}

func (q *qingCloudLoadBalanceExecutor) CreateBackends(input *qcservice.AddLoadBalancerBackendsInput) error {
	output, err := q.lbapi.AddLoadBalancerBackends(input)
	if err != nil {
		klog.Errorln("Failed to create backends")
		return newServerErrorOfBackend(*input.Backends[0].LoadBalancerBackendName, "CreateBackends", err)
	}
	if *output.RetCode != 0 {
		return errors.NewCommonServerError(ResourceNameBackend, *input.Backends[0].LoadBalancerBackendName, "CreateBackends", *output.Message)
	}
	return nil
}

func (q *qingCloudLoadBalanceExecutor) ModifyBackend(id string, weight int, port int) error {
	input := &qcservice.ModifyLoadBalancerBackendAttributesInput{
		LoadBalancerBackend: &id,
		Port:                &port,
		Weight:              &weight,
	}
	output, err := q.lbapi.ModifyLoadBalancerBackendAttributes(input)
	if err != nil {
		return newServerErrorOfBackend(id, "ModifyBackend", err)
	}
	if *output.RetCode != 0 {
		return errors.NewCommonServerError(ResourceNameBackend, id, "ModifyBackends", *output.Message)
	}
	return nil
}

func (q *qingCloudLoadBalanceExecutor) GetBackendsOfListener(id string) ([]*qcservice.LoadBalancerBackend, error) {
	input := &qcservice.DescribeLoadBalancerBackendsInput{
		LoadBalancerListener: &id,
	}
	output, err := q.lbapi.DescribeLoadBalancerBackends(input)
	if err != nil {
		return nil, newServerErrorOfListener(id, " GetBackendsOfListener", err)
	}
	return output.LoadBalancerBackendSet, nil
}

func (q *qingCloudLoadBalanceExecutor) GetBackendByID(id string) (*qcservice.LoadBalancerBackend, error) {
	input := &qcservice.DescribeLoadBalancerBackendsInput{
		LoadBalancerBackends: []*string{&id},
	}
	output, err := q.lbapi.DescribeLoadBalancerBackends(input)
	if err != nil {
		return nil, newServerErrorOfBackend(id, "GetBackendByID", err)
	}
	if len(output.LoadBalancerBackendSet) < 1 {
		return nil, errors.NewResourceNotFoundError(ResourceNameBackend, id)
	}
	return output.LoadBalancerBackendSet[0], nil
}
