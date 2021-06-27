package timer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"
	"github.com/toolkits/pkg/logger"
)

func SyncClasspathReses() {
	err := syncClasspathReses()
	if err != nil {
		fmt.Println("timer: sync classpath resources fail:", err)
		exit(1)
	}

	go loopSyncResClasspath()
}

func loopSyncResClasspath() {
	randtime := rand.Intn(10000)
	fmt.Printf("timer: sync classpath resources: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	interval := time.Duration(60) * time.Second

	for {
		time.Sleep(interval)

		err := syncClasspathReses()
		if err != nil {
			logger.Warning("timer: sync classpath resources fail:", err)
		}
	}
}

func syncClasspathReses() error {
	start := time.Now()

	classpaths, err := models.ClasspathGetAll()
	if err != nil {
		return err
	}

	// classpath_id -> classpath
	classpathMap := make(map[int64]*models.Classpath)
	for i := range classpaths {
		classpathMap[classpaths[i].Id] = &classpaths[i]
	}

	classpathResource, err := models.ClasspathResourceGetAll()
	if err != nil {
		return err
	}

	// classpath_id -> ident1, ident2 ...
	classpathRes := make(map[int64]*cache.ClasspathAndRes)

	// ident -> classpath1, classpath2 ...
	resClasspath := make(map[string]map[string]struct{})

	for _, cr := range classpathResource {
		c, has := classpathMap[cr.ClasspathId]
		if !has {
			// 理论上不会走到这里，只是做个防御
			continue
		}

		classpathAndRes, exists := classpathRes[cr.ClasspathId]
		if !exists {
			classpathRes[cr.ClasspathId] = &cache.ClasspathAndRes{
				Res:       []string{cr.ResIdent},
				Classpath: c,
			}
		} else {
			classpathAndRes.Res = append(classpathAndRes.Res, cr.ResIdent)
		}

		cset, exists := resClasspath[cr.ResIdent]
		if !exists {
			resClasspath[cr.ResIdent] = map[string]struct{}{
				c.Path: {},
			}
		} else {
			cset[c.Path] = struct{}{}
		}
	}

	cache.ClasspathRes.SetAll(classpathRes)
	cache.ResClasspath.SetAll(resClasspath)
	logger.Debugf("timer: sync classpath resources done, cost: %dms", time.Since(start).Milliseconds())

	return nil
}
