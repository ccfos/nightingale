package ormx

import (
	"context"
	"database/sql"
	"strings"

	"gorm.io/gorm"
)

// 达梦有两种自增形态，行为并不等价：
//
//	IDENTITY(1, 1)  —— gorm 建表时生成的形态。往这种列里写显式值，必须先
//	                   SET IDENTITY_INSERT <表> ON，否则报 -2723。
//	AUTO_INCREMENT  —— 从 MySQL 迁移过来的形态。这种列本来就允许写显式值，
//	                   而 SET IDENTITY_INSERT 对它无效，直接报
//	                   -2717 表[xxx]不存在IDENTITY列。
//
// gorm-dameng 的 Create 只看模型（主键是自增且被赋了非零值）就无条件发
// SET IDENTITY_INSERT，不看列的真实形态；该语句失败会中断整条 INSERT。于是同一份代码
// 在自建库上正常、在迁移库上所有"显式指定主键再插入"的写入全部失败——数据源同步就是
// 典型受害者，而且对调用方完全静默（接口报成功、数据没落库）。
//
// 这里在连接池层把这条语句的 -2717 吞掉：对 AUTO_INCREMENT 形态而言它本就多余，
// 失败无害，后续 INSERT 会照常执行；对 IDENTITY 形态它会成功，不受影响。
// 只针对 SET IDENTITY_INSERT 这一条语句、且只吞 -2717 这一个错误码，不影响其它语句。

// dmIdentityInsertPool 包装 *sql.DB。
// 注意 BeginTx 的签名与 *sql.DB 的不同，会遮蔽被提升的方法，使本类型只满足
// gorm.ConnPoolBeginner 而不满足 gorm.TxBeginner——gorm 的 Begin 优先匹配后者，
// 这样才能保证事务内的语句也走到包装过的 ExecContext。
type dmIdentityInsertPool struct {
	*sql.DB
}

func (p *dmIdentityInsertPool) GetDBConn() (*sql.DB, error) { return p.DB, nil }

func (p *dmIdentityInsertPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &dmIdentityInsertTx{tx}, nil
}

func (p *dmIdentityInsertPool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return dmExecTolerateIdentityInsert(ctx, p.DB, query, args...)
}

// dmIdentityInsertTx 让事务内的语句同样经过处理。Commit/Rollback 由 *sql.Tx 提升，
// 因此本类型同时满足 gorm.ConnPool 与 gorm.TxCommitter。
//
// 必须以指针形式使用：gorm 的 Commit/Rollback 会对 TxCommitter 做
// reflect.ValueOf(x).IsNil()，而 IsNil 对结构体值会 panic。
type dmIdentityInsertTx struct {
	*sql.Tx
}

func (t *dmIdentityInsertTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return dmExecTolerateIdentityInsert(ctx, t.Tx, query, args...)
}

type dmExecer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func dmExecTolerateIdentityInsert(ctx context.Context, e dmExecer, query string, args ...interface{}) (sql.Result, error) {
	res, err := e.ExecContext(ctx, query, args...)
	if err != nil && isDMSetIdentityInsert(query) && isDMNoIdentityColumn(err) {
		return dmNoopResult{}, nil
	}
	return res, err
}

func isDMSetIdentityInsert(query string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "SET IDENTITY_INSERT")
}

// -2717 是达梦的「表[xxx]不存在IDENTITY列」。只认这一个错误码，其它一律照常报出去。
func isDMNoIdentityColumn(err error) bool {
	return err != nil && strings.Contains(err.Error(), "-2717")
}

type dmNoopResult struct{}

func (dmNoopResult) LastInsertId() (int64, error) { return 0, nil }
func (dmNoopResult) RowsAffected() (int64, error) { return 0, nil }
