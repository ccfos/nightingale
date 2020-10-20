package models

import "xorm.io/xorm"

type NodeAdmin struct {
	NodeId int64
	UserId int64
}

func NodeAdminNew(session *xorm.Session, nodeId, userId int64) error {
	has, err := NodeAdminExists(session, nodeId, userId)
	if err != nil {
		return err
	}

	if has {
		return nil
	}

	na := NodeAdmin{
		NodeId: nodeId,
		UserId: userId,
	}

	_, err = session.Insert(&na)
	return err
}

func NodeAdminExists(session *xorm.Session, nodeId, userId int64) (bool, error) {
	num, err := session.Where("node_id=? and user_id=?", nodeId, userId).Count(new(NodeAdmin))
	return num > 0, err
}

func NodeClearAdmins(session *xorm.Session, nodeId int64) error {
	_, err := session.Where("node_id=?", nodeId).Delete(new(NodeAdmin))
	return err
}

func NodesAdminExists(nodeIds []int64, userId int64) (bool, error) {
	num, err := DB["rdb"].Where("user_id=?", userId).In("node_id", nodeIds).Count(new(NodeAdmin))
	return num > 0, err
}

// NodeIdsIamAdmin 我是管理员的节点ID列表
func NodeIdsIamAdmin(userId int64) ([]int64, error) {
	var ids []int64
	err := DB["rdb"].Table(new(NodeAdmin)).Where("user_id=?", userId).Select("node_id").Find(&ids)
	return ids, err
}
