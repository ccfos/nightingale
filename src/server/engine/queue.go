package engine

import "github.com/toolkits/pkg/container/list"

var EventQueue = list.NewSafeListLimited(10000000)
