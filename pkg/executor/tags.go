package executor

import (
	"fmt"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-sdk-go/service"
)

func (i *QingCloudClient) attachTagsToResources(resourceIDs []*string, resourceType string) error {
	if len(i.Config.TagIDs) <= 0 {
		return nil
	}

	input := &service.AttachTagsInput{}
	for _, tag := range i.Config.TagIDs {
		for _, resourceID := range resourceIDs {
			p := &service.ResourceTagPair{
				ResourceID:   resourceID,
				TagID:        service.String(tag),
				ResourceType: &resourceType,
			}
			input.ResourceTagPairs = append(input.ResourceTagPairs, p)
		}
	}
	output, err := i.tagService.AttachTags(input)
	if err != nil {
		return errors.NewCommonServerError("tag", fmt.Sprintf("%v", resourceIDs), "attachTagsToResources", err.Error())
	}
	if *output.RetCode != 0 {
		return errors.NewCommonServerError("tag", fmt.Sprintf("%v", resourceIDs), "attachTagsToResources", *output.Message)
	}
	return nil
}
