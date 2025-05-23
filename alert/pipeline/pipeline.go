package pipeline

import (
	_ "github.com/ccfos/nightingale/v6/alert/pipeline/processor/callback"
	_ "github.com/ccfos/nightingale/v6/alert/pipeline/processor/eventdrop"
	_ "github.com/ccfos/nightingale/v6/alert/pipeline/processor/eventupdate"
	_ "github.com/ccfos/nightingale/v6/alert/pipeline/processor/relabel"
)

func Init() {
}
