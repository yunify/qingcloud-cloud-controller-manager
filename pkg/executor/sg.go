package executor

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	qcclient "github.com/yunify/qingcloud-sdk-go/client"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog/v2"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
)

const (
	DefaultSgName = "k8s_lb_default_sg"
)

var defaultLBSecurityGroupRules = []*qcservice.SecurityGroupRule{
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

func convertSecurityGroup(sg *qcservice.SecurityGroup) *apis.SecurityGroup {
	return &apis.SecurityGroup{
		Spec: apis.SecurityGroupSpec{
			SecurityGroupName: sg.SecurityGroupName,
		},
		Status: apis.SecurityGroupStatus{
			SecurityGroupID: sg.SecurityGroupID,
		},
	}
}

func (q *QingCloudClient) CreateSecurityGroup(input *apis.SecurityGroup) (*apis.SecurityGroup, error) {
	createInput := &qcservice.CreateSecurityGroupInput{
		SecurityGroupName: input.Spec.SecurityGroupName,
	}
	createOutput, err := q.securityGroupService.CreateSecurityGroup(createInput)
	if err != nil || *createOutput.RetCode != 0 {
		klog.V(4).Infof("failed to create sg, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(createInput), spew.Sdump(createOutput))
		if err != nil {
			return nil, fmt.Errorf("failed to create sg, err=%v", err)
		}
		return nil, fmt.Errorf("failed to create sg, code=%d, msg=%s", *createOutput.RetCode, *createOutput.Message)
	}

	input.Status.SecurityGroupID = createOutput.SecurityGroupID

	err = q.attachTagsToResources([]*string{createOutput.SecurityGroupID}, SGTagResourceType)
	if err != nil {
		klog.Errorf("Failed to attach tag to security group %s, err: %v", *createOutput.SecurityGroupID, err)
	}

	return input, nil
}

// CreateSecurityGroup create a SecurityGroup in qingcloud
func (q *QingCloudClient) addSecurityGroupRules(sg *apis.SecurityGroup, rules []*qcservice.SecurityGroupRule) (*apis.SecurityGroup, error) {
	addRuleInput := &qcservice.AddSecurityGroupRulesInput{
		SecurityGroup: sg.Status.SecurityGroupID,
		Rules:         rules,
	}
	addRuleOutput, err := q.securityGroupService.AddSecurityGroupRules(addRuleInput)
	if err != nil || *addRuleOutput.RetCode != 0 {
		klog.V(4).Infof("failed to add sg rules, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(addRuleInput), spew.Sdump(addRuleOutput))
		if err != nil {
			return nil, fmt.Errorf("failed to add sg rules, err=%v", err)
		}
		return nil, fmt.Errorf("failed to add sg rules, code=%d, msg=%s", *addRuleOutput.RetCode, *addRuleOutput.Message)
	}

	sg.Status.SecurityGroupRuleIDs = addRuleOutput.SecurityGroupRules

	//You can put the returned jobid in the status field, and then the controller
	//will get the status at the same time.
	applyRuleInput := &qcservice.ApplySecurityGroupInput{
		SecurityGroup: sg.Status.SecurityGroupID,
	}
	applyRuleOutput, err := q.securityGroupService.ApplySecurityGroup(applyRuleInput)
	if err != nil || *applyRuleOutput.RetCode != 0 {
		klog.V(4).Infof("failed to apply sg rules, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(applyRuleInput), spew.Sdump(applyRuleOutput))
		if err != nil {
			return nil, fmt.Errorf("failed to apply sg rules, err=%v", err)
		}
		return nil, fmt.Errorf("failed to apply sg rules, code=%d, msg=%s", *applyRuleOutput.RetCode, *applyRuleOutput.Message)
	}

	err = qcclient.WaitJob(q.jobService, *applyRuleOutput.JobID, operationWaitTimeout, sgWaitInterval)
	if err != nil {
		klog.V(4).Infof("failed to apply sg rules, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(applyRuleInput), spew.Sdump(applyRuleOutput))
		return nil, fmt.Errorf("failed to apply sg rules, err=%v", err)
	}

	return sg, nil
}

func (q *QingCloudClient) DeleteSG(sg *string) error {
	input := &qcservice.DeleteSecurityGroupsInput{
		SecurityGroups: []*string{sg},
	}
	output, err := q.securityGroupService.DeleteSecurityGroups(input)
	if err != nil || *output.RetCode != 0 {
		klog.V(4).Infof("failed to delete sg %s, err=%s, output=%s", *sg, spew.Sdump(err), spew.Sdump(output))
		if err != nil {
			return fmt.Errorf("failed to delete sg %s, err=%v", *sg, err)
		}
		return fmt.Errorf("failed to delete sg %s, code=%d, msg=%s", *sg, *output.RetCode, *output.Message)
	}

	return nil
}

// Currently all load balancers that do not specify sg are using the default.
func (q *QingCloudClient) ensureSecurityGroupByName(name string) (*apis.SecurityGroup, error) {
	sg, err := q.GetSecurityGroupByName(name)
	if err != nil {
		if errors.IsResourceNotFound(err) {
			sg, err = q.CreateSecurityGroup(&apis.SecurityGroup{
				Spec: apis.SecurityGroupSpec{
					SecurityGroupName: &name,
				},
			})
			if err != nil {
				return nil, fmt.Errorf("create sg error: %v", err)
			}

			sg, err = q.addSecurityGroupRules(sg, defaultLBSecurityGroupRules)
			if err != nil {
				return nil, fmt.Errorf("add sg rules error: %v", err)
			}
		} else {
			return nil, fmt.Errorf("get sg by name error: %v", err)
		}
	}

	return sg, nil
}

func (q *QingCloudClient) GetSecurityGroupByName(name string) (*apis.SecurityGroup, error) {
	input := &qcservice.DescribeSecurityGroupsInput{
		SearchWord: &name,
		Owner:      &q.Config.UserID,
	}
	output, err := q.securityGroupService.DescribeSecurityGroups(input)
	if err != nil || *output.RetCode != 0 {
		klog.V(4).Infof("get sg by name %s failed, err=%s, input=%s, output=%s", name, spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
		if err != nil {
			return nil, fmt.Errorf("get sg by name %s failed, err=%v", name, err)
		}
		return nil, fmt.Errorf("get sg by name %s failed, code=%d, msg=%s", name, *output.RetCode, *output.Message)
	}

	if len(output.SecurityGroupSet) > 1 {
		klog.Warningf("more than one sg found by name %s, output=%s", name, spew.Sdump(output))
	}

	for _, sg := range output.SecurityGroupSet {
		if sg.SecurityGroupName != nil && *sg.SecurityGroupName == name {
			return convertSecurityGroup(sg), nil
		}
	}

	return nil, errors.NewResourceNotFoundError(ResourceNameSecurityGroup, name)
}
