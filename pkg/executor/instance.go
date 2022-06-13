package executor

import (
	"fmt"

	qcservice "github.com/yunify/qingcloud-sdk-go/service"
)

func (i *QingCloudClient) InstancesExist(ids []string) error {
	status := []*string{qcservice.String("pending"), qcservice.String("running"), qcservice.String("stopped")}
	input := &qcservice.DescribeInstancesInput{
		Instances: qcservice.StringSlice(ids),
		Status:    status,
		Verbose:   qcservice.Int(1),
	}
	if i.Config.IsAPP {
		input.IsClusterNode = qcservice.Int(1)
	}
	output, err := i.InstanceService.DescribeInstances(input)
	if err != nil {
		return err
	}
	if len(output.InstanceSet) != len(ids) {
		return fmt.Errorf("cannot find instances by node ids= %v", ids)
	}

	return nil
}
