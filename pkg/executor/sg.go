package executor

import (
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog"
)

var DefaultLBSecurityGroupRules = []*qcservice.SecurityGroupRule{
	{
		Priority: qcservice.Int(0),
		Protocol: qcservice.String("icmp"),
		Action:   qcservice.String("accept"),
		Val1:     qcservice.String("8"), //Echo
		Val2:     qcservice.String("0"), //Echo request
		Val3:     nil,
	},
	//allow all tcp port, lb only open listener's port,
	//security group is for limit ip source.
	{
		Priority: qcservice.Int(1),
		Protocol: qcservice.String("tcp"),
		Action:   qcservice.String("accept"),
		Val1:     qcservice.String("1"),
		Val2:     qcservice.String("65535"),
		Val3:     nil,
	},
	{
		Priority: qcservice.Int(1),
		Protocol: qcservice.String("udp"),
		Action:   qcservice.String("accept"),
		Val1:     qcservice.String("1"),
		Val2:     qcservice.String("65535"),
		Val3:     nil,
	},
}
var _ QingCloudSecurityGroupExecutor = &qingcloudSecurityGroupExecutor{}

func NewQingCloudSecurityGroupExecutor(sgapi *qcservice.SecurityGroupService) QingCloudSecurityGroupExecutor {
	return &qingcloudSecurityGroupExecutor{
		sgapi: sgapi,
	}
}

type qingcloudSecurityGroupExecutor struct {
	sgapi *qcservice.SecurityGroupService
}

func (q *qingcloudSecurityGroupExecutor) GetSecurityGroupByName(name string) (*qcservice.SecurityGroup, error) {
	input := &qcservice.DescribeSecurityGroupsInput{SearchWord: &name}
	output, err := q.sgapi.DescribeSecurityGroups(input)
	if err != nil {
		return nil, errors.NewCommonServerError(ResourceNameSecurityGroup, name, "GetSecurityGroupByName", err.Error())
	}
	for _, sg := range output.SecurityGroupSet {
		if sg.SecurityGroupName != nil && *sg.SecurityGroupName == name {
			return sg, nil
		}
	}
	return nil, errors.NewResourceNotFoundError(ResourceNameSecurityGroup, name)
}

// CreateSecurityGroup create a SecurityGroup in qingcloud
func (q *qingcloudSecurityGroupExecutor) CreateSecurityGroup(sgName string, rules []*qcservice.SecurityGroupRule) (*qcservice.SecurityGroup, error) {
	createInput := &qcservice.CreateSecurityGroupInput{SecurityGroupName: &sgName}
	createOutput, err := q.sgapi.CreateSecurityGroup(createInput)
	if err != nil {
		return nil, errors.NewCommonServerError(ResourceNameSecurityGroup, sgName, "CreateSecurityGroup", err.Error())
	}
	sgID := createOutput.SecurityGroupID
	addRuleOutput, err := q.sgapi.AddSecurityGroupRules(&qcservice.AddSecurityGroupRulesInput{SecurityGroup: sgID, Rules: rules})
	if err != nil {
		return nil, errors.NewCommonServerError(ResourceNameSecurityGroup, sgName, "AddSecurityGroupRules", err.Error())
	}
	klog.V(4).Infof("AddSecurityGroupRules SecurityGroup: [%s], output: [%+v] ", *sgID, addRuleOutput)
	o, err := q.sgapi.ApplySecurityGroup(&qcservice.ApplySecurityGroupInput{SecurityGroup: sgID})
	if err != nil {
		return nil, errors.NewCommonServerError(ResourceNameSecurityGroup, sgName, "ApplySecurityGroupRules", err.Error())
	}
	if *o.RetCode != 0 {
		return nil, errors.NewCommonServerError(ResourceNameSecurityGroup, sgName, "ApplySecurityGroupRules", *o.Message)
	}
	sg, _ := q.GetSecurityGroupByID(*sgID)
	return sg, nil
}

func (q *qingcloudSecurityGroupExecutor) EnsureSecurityGroup(name string) (*qcservice.SecurityGroup, error) {
	sg, err := q.GetSecurityGroupByName(name)
	if err != nil {
		if errors.IsResourceNotFound(err) {
			sg, err = q.CreateSecurityGroup(name, DefaultLBSecurityGroupRules)
			if err == nil {
				return sg, nil
			}
		}
		return nil, errors.NewCommonServerError(ResourceNameSecurityGroup, name, "EnsureSecurityGroup", err.Error())
	}
	return sg, nil
}

// GetSecurityGroupByID return SecurityGroup in qingcloud using ID
func (q *qingcloudSecurityGroupExecutor) GetSecurityGroupByID(id string) (*qcservice.SecurityGroup, error) {
	input := &qcservice.DescribeSecurityGroupsInput{SecurityGroups: []*string{&id}}
	output, err := q.sgapi.DescribeSecurityGroups(input)
	if err != nil {
		return nil, errors.NewCommonServerError(ResourceNameSecurityGroup, id, "GetSecurityGroupByID", err.Error())
	}
	if len(output.SecurityGroupSet) > 0 {
		return output.SecurityGroupSet[0], nil
	}
	return nil, errors.NewResourceNotFoundError(ResourceNameSecurityGroup, id)
}

func (q *qingcloudSecurityGroupExecutor) GetSgAPI() *qcservice.SecurityGroupService {
	return q.sgapi
}

func (q *qingcloudSecurityGroupExecutor) Delete(id string) error {
	input := &qcservice.DeleteSecurityGroupsInput{SecurityGroups: []*string{&id}}
	_, err := q.sgapi.DeleteSecurityGroups(input)
	if err != nil {
		return errors.NewCommonServerError(ResourceNameSecurityGroup, id, "Delete", err.Error())
	}
	return nil
}
