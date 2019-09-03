package fake

import (
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/e2eutil"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
)

type FakeSecurityGroupExecutor struct {
	SecurityGroups map[string]*qcservice.SecurityGroup
	TagIDs         []string
}

func NewFakeSecurityGroupExecutor() *FakeSecurityGroupExecutor {
	f := new(FakeSecurityGroupExecutor)
	f.SecurityGroups = make(map[string]*qcservice.SecurityGroup)
	return f
}

func (f *FakeSecurityGroupExecutor) GetSecurityGroupByName(name string) (*qcservice.SecurityGroup, error) {
	for _, v := range f.SecurityGroups {
		if *v.SecurityGroupName == name {
			return v, nil
		}
	}
	return nil, errors.NewResourceNotFoundError(executor.ResourceNameSecurityGroup, name)
}

func (f *FakeSecurityGroupExecutor) CreateSecurityGroup(sgName string, _ []*qcservice.SecurityGroupRule) (*qcservice.SecurityGroup, error) {
	sg := new(qcservice.SecurityGroup)
	id := "sg-" + e2eutil.RandString(8)
	sg.SecurityGroupID = &id
	sg.SecurityGroupName = &sgName
	f.SecurityGroups[id] = sg
	return sg, nil
}

func (f *FakeSecurityGroupExecutor) EnsureSecurityGroup(name string) (*qcservice.SecurityGroup, error) {
	if s, _ := f.GetSecurityGroupByName(name); s != nil {
		return s, nil
	}
	return f.CreateSecurityGroup(name, nil)
}

func (f *FakeSecurityGroupExecutor) GetSecurityGroupByID(id string) (*qcservice.SecurityGroup, error) {
	return f.SecurityGroups[id], nil
}

func (f *FakeSecurityGroupExecutor) GetSgAPI() *qcservice.SecurityGroupService {
	return nil
}

func (f *FakeSecurityGroupExecutor) Delete(id string) error {
	delete(f.SecurityGroups, id)
	return nil
}

func (f *FakeSecurityGroupExecutor) EnableTagService(ids []string) {
	f.TagIDs = ids
}
