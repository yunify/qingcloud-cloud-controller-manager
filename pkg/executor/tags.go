package executor

import (
	"fmt"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-sdk-go/service"
)

func AttachTagsToResources(tagapi *service.TagService, tags []string, resourceIDs []string, resourceType string) error {
	if len(tags) <= 0 {
		return nil
	}

	input := &service.AttachTagsInput{}
	for _, tag := range tags {
		for _, resourceID := range resourceIDs {
			p := &service.ResourceTagPair{
				ResourceID:   &resourceID,
				TagID:        service.String(tag),
				ResourceType: &resourceType,
			}
			input.ResourceTagPairs = append(input.ResourceTagPairs, p)
		}
	}
	output, err := tagapi.AttachTags(input)
	if err != nil {
		return errors.NewCommonServerError("tag", fmt.Sprintf("%v", resourceIDs), "AttachTagsToResources", err.Error())
	}
	if *output.RetCode != 0 {
		return errors.NewCommonServerError("tag", fmt.Sprintf("%v", resourceIDs), "AttachTagsToResources", *output.Message)
	}
	return nil
}

func DetachTagsFromResources(tagapi *service.TagService, tags []string, resourceIDs []string, resourceType string) error {
	if len(tags) <= 0 {
		return nil
	}

	input := &service.DetachTagsInput{}
	for _, tag := range tags {
		for _, resourceID := range resourceIDs {
			p := &service.ResourceTagPair{
				ResourceID:   &resourceID,
				TagID:        service.String(tag),
				ResourceType: &resourceType,
			}
			input.ResourceTagPairs = append(input.ResourceTagPairs, p)
		}
	}
	output, err := tagapi.DetachTags(input)
	if err != nil {
		return errors.NewCommonServerError("tag", fmt.Sprintf("%v", resourceIDs), "DetachTagsFromResources", err.Error())
	}
	if *output.RetCode != 0 {
		return errors.NewCommonServerError("tag", fmt.Sprintf("%v", resourceIDs), "DetachTagsFromResources", *output.Message)
	}
	return nil
}
