/*
Copyright 2023 Reactive Tech Limited.
"Reactive Tech Limited" is a company located in England, United Kingdom.
https://www.reactive-tech.io

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package statefulset_spec

import (
	"strconv"

	apps "k8s.io/api/apps/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
)

const defaultTerminationGracePeriodSeconds int64 = 10

type TerminationGracePeriodSecondsSpecEnforcer struct {
	kubegresContext ctx.KubegresContext
}

func CreateTerminationGracePeriodSecondsSpecEnforcer(kubegresContext ctx.KubegresContext) TerminationGracePeriodSecondsSpecEnforcer {
	return TerminationGracePeriodSecondsSpecEnforcer{kubegresContext: kubegresContext}
}

func (r *TerminationGracePeriodSecondsSpecEnforcer) GetSpecName() string {
	return "TerminationGracePeriodSeconds"
}

func (r *TerminationGracePeriodSecondsSpecEnforcer) expected() int64 {
	if value := r.kubegresContext.Kubegres.Spec.TerminationGracePeriodSeconds; value != nil {
		return *value
	}
	return defaultTerminationGracePeriodSeconds
}

func (r *TerminationGracePeriodSecondsSpecEnforcer) CheckForSpecDifference(statefulSet *apps.StatefulSet) StatefulSetSpecDifference {
	current := statefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds
	expected := r.expected()
	if current == nil || *current != expected {
		currentValue := "<nil>"
		if current != nil {
			currentValue = strconv.FormatInt(*current, 10)
		}
		return StatefulSetSpecDifference{
			SpecName: r.GetSpecName(),
			Current:  currentValue,
			Expected: strconv.FormatInt(expected, 10),
		}
	}
	return StatefulSetSpecDifference{}
}

func (r *TerminationGracePeriodSecondsSpecEnforcer) EnforceSpec(statefulSet *apps.StatefulSet) (bool, error) {
	expected := r.expected()
	statefulSet.Spec.Template.Spec.TerminationGracePeriodSeconds = &expected
	return true, nil
}

func (r *TerminationGracePeriodSecondsSpecEnforcer) OnSpecEnforcedSuccessfully(statefulSet *apps.StatefulSet) error {
	return nil
}
