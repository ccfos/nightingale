package dataobj

type StraData struct {
	Dat []Stra `json:"dat"`
	Err string `json:"err"`
}

type Stra struct {
	ID               int64        `json:"id"`
	Name             string       `json:"name"`
	Category         int          `json:"category"`
	Nid              int64        `json:"nid"`
	AlertDur         int          `json:"alert_dur"`
	RecoveryDur      int          `json:"recovery_dur"`
	EnableStime      string       `json:"enable_stime"`
	EnableEtime      string       `json:"enable_etime"`
	Priority         int          `json:"priority"`
	Callback         string       `json:"callback"`
	Creator          string       `json:"creator"`
	Created          string       `json:"created"`
	LastUpdator      string       `json:"last_updator"`
	LastUpdated      string       `json:"last_updated"`
	ExclNid          []int64      `json:"excl_nid"`
	Exprs            []Exp        `json:"exprs"`
	Tags             []Tag        `json:"tags"`
	EnableDaysOfWeek []int        `json:"enable_days_of_week"`
	Converge         []int        `json:"converge"`
	RecoveryNotify   int          `json:"recovery_notify"`
	NotifyGroup      []int64      `json:"notify_group"`
	NotifyUser       []int64      `json:"notify_user"`
	LeafNids         interface{}  `json:"leaf_nids"`
	NeedUpgrade      int          `json:"need_upgrade"`
	AlertUpgrade     AlertUpgrade `json:"alert_upgrade"`
}

type Exp struct {
	Eopt      string  `json:"eopt"`
	Func      string  `json:"func"`
	Metric    string  `json:"metric"`
	Params    []int   `json:"params"`
	Threshold float64 `json:"threshold"`
}

type Tag struct {
	Tkey string   `json:"tkey"`
	Topt string   `json:"topt"`
	Tval []string `json:"tval"` //修改为数组
}

type AlertUpgrade struct {
	Users    []int64 `json:"users"`
	Groups   []int64 `json:"groups"`
	Duration int     `json:"duration"`
	Level    int     `json:"level"`
}
