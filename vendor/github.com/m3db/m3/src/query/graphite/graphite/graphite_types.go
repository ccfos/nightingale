// Copyright (c) 2019 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package graphite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/hydrogen18/stalecucumber"
)

// MIMETypeApplicationPickle defines the MIME type for application/pickle content
const MIMETypeApplicationPickle = "application/pickle"

// A Timestamp is a time.Time that knows how to marshal and unmarshal
// itself as Graphite expects (as seconds since Unix epoch)
type Timestamp time.Time

// MarshalJSON marshals the timestamp as JSON
func (t Timestamp) MarshalJSON() ([]byte, error) {
	s := strconv.FormatInt(time.Time(t).Unix(), 10)
	return []byte(s), nil
}

// UnmarshalJSON unmarshals the timestamp from JSON
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	n, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}

	*t = Timestamp(time.Unix(n, 0).UTC())
	return nil
}

// A Datavalue is a float64 which knows how to marshal and unmarshal
// itself as Graphite expects (NaNs becomes nulls)
type Datavalue float64

// MarshalJSON marshals the value as JSON, writing NaNs as nulls
func (v Datavalue) MarshalJSON() ([]byte, error) {
	if math.IsNaN(float64(v)) {
		return []byte("null"), nil
	}

	s := strconv.FormatFloat(float64(v), 'f', -1, 64)
	return []byte(s), nil
}

// UnmarshalJSON unmarshals the value as JSON, converting nulls into NaNs
func (v *Datavalue) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" {
		*v = Datavalue(math.NaN())
		return nil
	}

	n, err := strconv.ParseFloat(s, 64)
	*v = Datavalue(n)
	return err
}

// RenderDatapoints are the set of datapoints returned from Graphite rendering
type RenderDatapoints [][]interface{}

// Add adds a new datapoint to the set of datapoints
func (dp *RenderDatapoints) Add(timestamp time.Time, value float64) {
	*dp = append(*dp, []interface{}{Datavalue(value), Timestamp(timestamp)})
}

// Get returns the timestamp and value at the given index
func (dp RenderDatapoints) Get(i int) (time.Time, float64) {
	value := math.NaN()
	if dp[i][0] != nil {
		value = dp[i][0].(float64)
	}

	switch timestamp := dp[i][1].(type) {
	case float64:
		return time.Unix(int64(timestamp), 0).UTC(), value
	case int:
		return time.Unix(int64(timestamp), 0).UTC(), value
	case time.Time:
		return timestamp, value
	default:
		panic(fmt.Sprintf("unsupported timestamp type"))
	}
}

// A RenderTarget is the result of rendering a given target
type RenderTarget struct {
	Target     string           `json:"target"`
	Datapoints RenderDatapoints `json:"datapoints"`
}

// RenderResults are the results from a render API call
type RenderResults []RenderTarget

// A Datapoint is a Timestamp/Value pair representing a single value in a
// target
type Datapoint struct {
	Timestamp Timestamp `json:"t"`
	Value     Datavalue `json:"v"`
}

// Results are a map of graphite target names to their corresponding datapoints
type Results map[string][]Datapoint

// RenderResultsPickle is an alternate form of graphite result, consisting of a
// start time, an end time, a step size (in seconds), and values for each step.
// Steps that do not have a value will be NaN
type RenderResultsPickle struct {
	Name   string        `pickle:"name"`
	Start  uint32        `pickle:"start"`
	End    uint32        `pickle:"end"`
	Step   uint32        `pickle:"step"`
	Values []interface{} `pickle:"values"` // value can be nil (python 'None')
}

// Len returns the number of results
func (p RenderResultsPickle) Len() int { return len(p.Values) }

// ValueAt returns the value at the given step
func (p RenderResultsPickle) ValueAt(n int) float64 {
	if p.Values[n] == nil {
		return math.NaN()
	}

	return p.Values[n].(float64)
}

// Get returns the timestamp and value at the given index
func (p RenderResultsPickle) Get(i int) (time.Time, float64) {
	value := math.NaN()
	if p.Values[i] != nil {
		value = p.Values[i].(float64)
	}

	timestamp := time.Unix(int64(p.Start)+int64(p.Step*uint32(i)), 0).UTC()
	return timestamp, value
}

// ParseRenderResultsPickle parses a byte stream containing a pickle render response
func ParseRenderResultsPickle(b []byte) ([]RenderResultsPickle, error) {
	r := bytes.NewReader(b)

	var pickleResults []RenderResultsPickle
	if err := stalecucumber.UnpackInto(&pickleResults).From(stalecucumber.Unpickle(r)); err != nil {
		return nil, err
	}

	//convert stalecucumber.PickleNone to nil
	for _, r := range pickleResults {
		for i, v := range r.Values {
			_, ok := v.(stalecucumber.PickleNone)
			if ok {
				r.Values[i] = nil
			}
		}
	}

	return pickleResults, nil
}

// ParseJSONResponse takes a byteBuffer and returns Results
func ParseJSONResponse(b []byte) (Results, error) {
	var jsonResults []jsonResult
	if err := json.Unmarshal(b, &jsonResults); err != nil {
		return nil, err
	}

	results := make(Results, len(jsonResults))
	for _, jsonResult := range jsonResults {
		datapoints := make([]Datapoint, 0, len(jsonResult.Datapoints))
		for _, jsonPoint := range jsonResult.Datapoints {
			if jsonPoint[0] == nil {
				jsonPoint[0] = math.NaN()
			}
			datapoints = append(datapoints, Datapoint{
				Timestamp: Timestamp(time.Unix(int64(jsonPoint[1].(float64)), 0)),
				Value:     Datavalue(jsonPoint[0].(float64)),
			})
		}

		results[jsonResult.Target] = datapoints
	}

	return results, nil
}

type jsonResult struct {
	Target     string          `json:"target"`
	Datapoints [][]interface{} `json:"datapoints"`
}

// RespondWithPickle sends a python pickle response
func RespondWithPickle(w http.ResponseWriter, data interface{}) error {
	w.Header().Add("Content-Type", MIMETypeApplicationPickle)
	var buf bytes.Buffer
	_, err := stalecucumber.NewPickler(&buf).Pickle(data)
	if err != nil {
		return err
	}

	_, err = w.Write(buf.Bytes())
	return err
}

// MetricsPathMetadata is an internal element of graphite's "completer" format
// for /metrics/find results.  sample: {"is_leaf": "1", "path":
// "servers.foo-bar.baz.qux_qaz", "name":
// "qux_qaz"}
type MetricsPathMetadata struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	IsLeaf int    `json:"is_leaf,string"` // UGLY(jayp): should be a bool, int due to encoding/json
}

// FindResultsPickle is graphite's pickle format for /metrics/find results
type FindResultsPickle struct {
	Path   string `pickle:"path" json:"path"`
	IsLeaf bool   `pickle:"is_leaf" json:"is_leaf"`
}

// FindResultsCompleterJSON is graphite's "completer" format for /metrics/find
// results sample: {"metrics": [...]}
type FindResultsCompleterJSON struct {
	Metrics []MetricsPathMetadata `json:"metrics"`
}

// FindResultsTreeJSON is graphite's "treeJSON" format for /metrics/find
// results.  sample: {"text": "quz", "expandable": 1, "leaf": 0, "id":
// "foo.bar-baz.qux.quz", "allowChildren": 1}
type FindResultsTreeJSON struct {
	ID            string `json:"id"`            // =path
	Text          string `json:"text"`          // =name
	Leaf          int    `json:"leaf"`          // =isLeaf
	Expandable    int    `json:"expandable"`    // =!isLeaf
	AllowChildren int    `json:"allowChildren"` // =!isLeaf
}
