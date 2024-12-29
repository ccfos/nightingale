package promql

import (
	"reflect"
	"testing"
)

func TestGetMetric(t *testing.T) {
	tests := []struct {
		name    string
		ql      string
		want    map[string]string
		wantErr error
	}{
		{
			name:    "Valid query with labels",
			ql:      "metric_name{label1=\"value1\",label2=\"value2\"}",
			want:    map[string]string{"metric_name": "metric_name{label1=\"value1\",label2=\"value2\"}"},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetMetric(tt.ql)
			if err != tt.wantErr && err != nil {
				t.Errorf("GetMetric() error = %v, wantErr %v ql:%s", err, tt.wantErr, tt.ql)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetMetric() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLabels(t *testing.T) {
	tests := []struct {
		name    string
		ql      string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "Valid query with multiple labels",
			ql:   "metric_name{label1=\"value1\", label2=\"value2\"} > 3",
			want: map[string]string{"label1": "value1", "label2": "value2"},
		},
		{
			name: "Valid query with multiple labels",
			ql:   "metric_name{label1=\"$value1\", label2=\"$value2\"} > 3",
			want: map[string]string{"label1": "$value1", "label2": "$value2"},
		},
		{
			name: "Query without labels",
			ql:   "metric_name",
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetLabels(tt.ql)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetLabels() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetLabels() = %v, want %v ql:%s", got, tt.want, tt.ql)
			}
		})
	}
}

func TestGetLabelsAndMetricNameWithReplace(t *testing.T) {
	// 定义测试案例
	tests := []struct {
		name               string
		ql                 string
		rep                string
		expectedLabels     map[string]Label
		expectedMetricName string
		expectError        bool
	}{
		{
			name: "正常情况",
			ql:   `(snmp_arista_system_cpuuse{ent_descr="$ent_descr"} / 100 > $cpu_high_threshold[1m])`,
			rep:  "$",
			expectedLabels: map[string]Label{
				"ent_descr": {Name: "ent_descr", Value: "$ent_descr", Op: "="},
			},
			expectedMetricName: "snmp_arista_system_cpuuse",
			expectError:        false,
		},
		{
			name: "正常情况",
			ql:   `rate(snmp_interface_incoming{agent_host='$agent_host',ifname='$ifname'}[2m]) * 8 / 10^9 > snmp_interface_speed{agent_host='$agent_host',ifname='$ifname'}/ 10^3 *  $traffic_in and snmp_interface_speed{agent_host='$agent_host',ifname='$ifname'} > 0`,
			rep:  "$",
			expectedLabels: map[string]Label{
				"agent_host": {Name: "agent_host", Value: "$agent_host", Op: "="},
				"ifname":     {Name: "ifname", Value: "$ifname", Op: "="},
			},
			expectedMetricName: "snmp_interface_speed",
			expectError:        false,
		},
		{
			name: "正常情况",
			ql:   `rate(snmp_interface_incoming{agent_host='$agent_host',ifname='$ifname'}[2m]) * 8 / 10^9 > snmp_interface_speed{agent_host='$agent_host',ifname='$ifname'}/ 10^3 *  $traffic_in`,
			rep:  "$",
			expectedLabels: map[string]Label{
				"agent_host": {Name: "agent_host", Value: "$agent_host", Op: "="},
				"ifname":     {Name: "ifname", Value: "$ifname", Op: "="},
			},
			expectedMetricName: "snmp_interface_speed",
			expectError:        false,
		},
		{
			name: "正常情况",
			ql:   `rate(snmp_interface_incoming{agent_host='$agent_host',ifname='$ifname'}[2m]) * 8 / 10^9 > 10`,
			rep:  "$",
			expectedLabels: map[string]Label{
				"agent_host": {Name: "agent_host", Value: "$agent_host", Op: "="},
				"ifname":     {Name: "ifname", Value: "$ifname", Op: "="},
			},
			expectedMetricName: "snmp_interface_incoming",
			expectError:        false,
		},
		{
			name: "带有替换字符",
			ql:   `rate(snmp_interface_outgoing{Role=~'ZRT.*',agent_host='$agent_host',ifname='$ifname'}[2m]) * 8 / 10^9 > snmp_interface_speed{Role=~'ZRT.*',agent_host='$agent_host',ifname='$ifname'}/ 10^3 * $outgoing_warning and snmp_interface_speed{Role=~'ZRT.*',agent_host='$agent_host',ifname='$ifname'} > 0`,
			rep:  "$",
			expectedLabels: map[string]Label{
				"agent_host": {Name: "agent_host", Value: "$agent_host", Op: "="},
				"ifname":     {Name: "ifname", Value: "$ifname", Op: "="},
				"Role":       {Name: "Role", Value: "ZRT.*", Op: "=~"},
			},
			expectedMetricName: "snmp_interface_speed",
			expectError:        false,
		},
		// 更多测试案例...
		{
			name:               "告警规则支持变量",
			ql:                 `mem{test1="$test1", test2="$test2", test3="test3"} > $val`,
			rep:                "$",
			expectedLabels:     map[string]Label{},
			expectedMetricName: "snmp_interface_speed",
			expectError:        false,
		},
	}

	// 运行测试案例
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			labels, metricName, err := GetLabelsAndMetricNameWithReplace(tc.ql, tc.rep)

			if (err != nil) != tc.expectError {
				t.Errorf("ql:%s 测试 '%v' 发生错误: %v, 期望的错误状态: %v", tc.ql, tc.name, err, tc.expectError)
			}

			if !reflect.DeepEqual(labels, tc.expectedLabels) {
				t.Errorf("ql:%s 测试 '%v' 返回的标签不匹配: got %v, want %v", tc.ql, tc.name, labels, tc.expectedLabels)
			}

			if metricName != tc.expectedMetricName {
				t.Errorf("ql:%s 测试 '%v' 返回的度量名称不匹配: got %s, want %s", tc.ql, tc.name, metricName, tc.expectedMetricName)
			}
		})
	}
}

func TestSplitBinaryOp(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		want    []string
		wantErr bool
	}{
		{
			name: "valid binary operation with spaces",
			code: "cpu_usage  +  memory_usage",
			want: []string{"cpu_usage + memory_usage"},
		},
		{
			name: "12",
			code: "cpu_usage > 0 and memory_usage>0",
			want: []string{"cpu_usage", "memory_usage"},
		},
		{
			name: "12",
			code: "cpu_usage +1> 0",
			want: []string{"cpu_usage + 1"},
		},
		{
			name: "valid complex binary operation",
			code: "(cpu_usage + memory_usage) / 2",
			want: []string{"(cpu_usage + memory_usage) / 2"},
		},
		{
			name:    "invalid binary operation",
			code:    "cpu_usage + ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitBinaryOp(tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("SplitBinaryOp() code:%s error = %v, wantErr %v", tt.code, err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitBinaryOp() got = %v, want %v", got, tt.want)
			}
		})
	}
}
