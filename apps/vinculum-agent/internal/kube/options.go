package kube

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func metaGetOptions() metav1.GetOptions   { return metav1.GetOptions{} }
func metaPatchOptions() metav1.PatchOptions { return metav1.PatchOptions{} }
