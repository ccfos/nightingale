package router

import "testing"

// 非 admin 只能把 MCP 授权给自己所属的团队 —— 这是「子集」，不是「有交集」。
//
// 旧实现拿 mcpCallerCanManage（交集）去校验请求体的团队归属，于是只要混入一个自己的
// 团队，就能把任意未加入的团队一起授权进去。黑盒实测：tester（仅属研发一组 id=2）
// POST /api/n9e/mcp-servers 传 [2,3] 成功落库，研发二组的成员真的收到了这台
// private=1 的 MCP。Skill 侧一直是子集校验（resolveSkillAuth），MCP 侧漏了。
//
// 下面的用例编号对应用户测试里的实际请求。
func TestMCPTeamAssignmentAllowed(t *testing.T) {
	const (
		mine    = 2 // 研发一组，tester 属于此
		foreign = 3 // 研发二组，tester 不属于
		other   = 1 // demo-root-group，tester 不属于
	)
	gids := []int64{mine}

	cases := []struct {
		name string
		prev []int64
		next []int64
		want bool
	}{
		// —— 创建（prev = nil）——
		{"① 仅未加入团队 [3] 必须拒绝", nil, []int64{foreign}, false},
		{"② 仅自己团队 [2] 允许", nil, []int64{mine}, true},
		{"③ 混入未加入团队 [2,3] 必须拒绝", nil, []int64{mine, foreign}, false},
		{"④ 混入未加入团队 [2,1] 必须拒绝", nil, []int64{mine, other}, false},
		// 非 admin 不能建无管理团队的 server（保持旧行为）
		{"空团队列表拒绝", nil, nil, false},

		// —— 编辑 —— 只有「新增的」团队需要是自己的
		{"保持既有的他组共有团队，允许", []int64{mine, foreign}, []int64{mine, foreign}, true},
		{"移除他组、只留自己，允许", []int64{mine, foreign}, []int64{mine}, true},
		{"新增未加入团队，拒绝", []int64{mine}, []int64{mine, foreign}, false},
		{"把自己踢出（自锁），拒绝", []int64{mine, foreign}, []int64{foreign}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mcpTeamAssignmentAllowed(gids, tc.prev, tc.next); got != tc.want {
				t.Fatalf("mcpTeamAssignmentAllowed(gids=%v, prev=%v, next=%v) = %v, want %v",
					gids, tc.prev, tc.next, got, tc.want)
			}
		})
	}
}

// 多团队用户：授权给自己所属的多个团队应当允许（子集校验不该误伤正常场景）。
func TestMCPTeamAssignmentMultiTeamUser(t *testing.T) {
	gids := []int64{1, 2}
	if !mcpTeamAssignmentAllowed(gids, nil, []int64{1, 2}) {
		t.Fatal("a user in teams [1,2] must be able to authorize both")
	}
	if !mcpTeamAssignmentAllowed(gids, nil, []int64{2}) {
		t.Fatal("a user in teams [1,2] must be able to authorize just one of them")
	}
	if mcpTeamAssignmentAllowed(gids, nil, []int64{1, 2, 3}) {
		t.Fatal("team 3 is not theirs — must be rejected even though 1 and 2 are")
	}
}
