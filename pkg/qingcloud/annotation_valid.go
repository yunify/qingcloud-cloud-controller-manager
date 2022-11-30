package qingcloud

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	ListenerTimeoutMin = 10
	ListenerTimeoutMax = 86400
)

func parseAnnotationIntoIntIntMap(annotationConfValue string) (map[int]int, error) {
	result := make(map[int]int)
	items := strings.Split(annotationConfValue, ",")
	if len(items) == 0 {
		return nil, nil
	}
	for _, item := range items {
		parts := strings.Split(item, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid format")
		}
		k, err1 := strconv.Atoi(parts[0])
		v, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("invalid value")
		}
		result[k] = v

	}
	return result, nil
}

func validListenerTimeout(annotationTimeoutConf string) error {
	result, err := parseAnnotationIntoIntIntMap(annotationTimeoutConf)
	if err != nil {
		return fmt.Errorf("invalid timeout conf: %v for nanotation %s ", err, ServiceAnnotationListenerTimeout)
	}

	// the value must in range 10 ～ 86400
	for _, value := range result {
		if value < ListenerTimeoutMin || value > ListenerTimeoutMax {
			return fmt.Errorf("invalid timeout conf: please spec a timeout value in range (%d, %d)", ListenerTimeoutMin, ListenerTimeoutMax)
		}
	}

	return nil
}
