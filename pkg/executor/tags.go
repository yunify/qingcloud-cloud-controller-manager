package executor

import (
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-sdk-go/service"
)

func AddTagsToResource(tagapi *service.TagService, tags []string, resourceID string, resourceType string) error {
	input := &service.AttachTagsInput{}
	for _, tag := range tags {
		p := &service.ResourceTagPair{
			ResourceID:   &resourceID,
			TagID:        service.String(tag),
			ResourceType: &resourceType,
		}
		input.ResourceTagPairs = append(input.ResourceTagPairs, p)
	}
	output, err := tagapi.AttachTags(input)
	if err != nil {
		return errors.NewCommonServerError("tag", resourceID, "AddTagsToResource", err.Error())
	}
	if *output.RetCode != 0 {
		return errors.NewCommonServerError("tag", resourceID, "AddTagsToResource", *output.Message)
	}
	return nil
}
