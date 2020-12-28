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

package namespace

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"strings"

	nsproto "github.com/m3db/m3/src/dbnode/generated/proto/namespace"
	xerrors "github.com/m3db/m3/src/x/errors"
	"github.com/m3db/m3/src/x/ident"

	"github.com/golang/protobuf/proto"
	dpb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
)

var (
	errInvalidSchema        = errors.New("invalid schema definition")
	errSchemaRegistryEmpty  = errors.New("schema registry is empty")
	errInvalidSchemaOptions = errors.New("invalid schema options")
	errEmptyProtoFile       = errors.New("empty proto file")
	errSyntaxNotProto3      = errors.New("proto syntax is not proto3")
	errEmptyDeployID        = errors.New("schema deploy ID can not be empty")
	errDuplicateDeployID    = errors.New("schema deploy ID already exists")
)

type MessageDescriptor struct {
	*desc.MessageDescriptor
}

type schemaDescr struct {
	deployId     string
	prevDeployId string
	md           MessageDescriptor
}

func newSchemaDescr(deployId, prevId string, md MessageDescriptor) *schemaDescr {
	return &schemaDescr{deployId: deployId, prevDeployId: prevId, md: md}
}

func (s *schemaDescr) DeployId() string {
	return s.deployId
}

func (s *schemaDescr) PrevDeployId() string {
	return s.prevDeployId
}

func (s *schemaDescr) Equal(o SchemaDescr) bool {
	if s == nil && o == nil {
		return true
	}
	if s != nil && o == nil || s == nil && o != nil {
		return false
	}
	return s.DeployId() == o.DeployId() && s.PrevDeployId() == o.PrevDeployId()
}

func (s *schemaDescr) Get() MessageDescriptor {
	return s.md
}

func (s *schemaDescr) String() string {
	if s.md.MessageDescriptor == nil {
		return ""
	}
	return s.md.MessageDescriptor.String()
}

type schemaHistory struct {
	options  *nsproto.SchemaOptions
	latestId string
	// a map of schema version to schema descriptor.
	versions map[string]*schemaDescr
}

func (sr *schemaHistory) Equal(o SchemaHistory) bool {
	var osr *schemaHistory
	var ok bool

	if osr, ok = o.(*schemaHistory); !ok {
		return false
	}
	// compare latest version
	if sr.latestId != osr.latestId {
		return false
	}

	// compare version map
	if len(sr.versions) != len(osr.versions) {
		return false
	}
	for v, sd := range sr.versions {
		osd, ok := osr.versions[v]
		if !ok {
			return false
		}
		if !sd.Equal(osd) {
			return false
		}
	}

	return true
}

func (sr *schemaHistory) Extends(v SchemaHistory) bool {
	cur, hasMore := v.GetLatest()

	for hasMore {
		srCur, inSr := sr.Get(cur.DeployId())
		if !inSr || !cur.Equal(srCur) {
			return false
		}
		cur, hasMore = v.Get(cur.PrevDeployId())
	}
	return true
}

func (sr *schemaHistory) Get(id string) (SchemaDescr, bool) {
	sd, ok := sr.versions[id]
	if !ok {
		return nil, false
	}
	return sd, true
}

func (sr *schemaHistory) GetLatest() (SchemaDescr, bool) {
	return sr.Get(sr.latestId)
}

// toSchemaOptions returns the corresponding SchemaOptions proto for the provided SchemaHistory
func toSchemaOptions(sr SchemaHistory) *nsproto.SchemaOptions {
	if sr == nil {
		return nil
	}
	_, ok := sr.(*schemaHistory)
	if !ok {
		return nil
	}
	return sr.(*schemaHistory).options
}

func emptySchemaHistory() SchemaHistory {
	return &schemaHistory{options: nil, versions: make(map[string]*schemaDescr)}
}

// LoadSchemaHistory loads schema registry from SchemaOptions proto.
func LoadSchemaHistory(options *nsproto.SchemaOptions) (SchemaHistory, error) {
	sr := &schemaHistory{options: options, versions: make(map[string]*schemaDescr)}
	if options == nil ||
		options.GetHistory() == nil ||
		len(options.GetHistory().GetVersions()) == 0 {
		return sr, nil
	}

	msgName := options.GetDefaultMessageName()
	if len(msgName) == 0 {
		return nil, xerrors.Wrap(errInvalidSchemaOptions, "default message name is not specified")
	}

	var prevId string
	for _, fdbSet := range options.GetHistory().GetVersions() {
		if len(prevId) > 0 && fdbSet.PrevId != prevId {
			return nil, xerrors.Wrapf(errInvalidSchemaOptions, "schema history is not sorted by deploy id in ascending order")
		}
		sd, err := loadFileDescriptorSet(fdbSet, msgName)
		if err != nil {
			return nil, err
		}
		sr.versions[sd.DeployId()] = sd
		prevId = sd.DeployId()
	}
	sr.latestId = prevId

	return sr, nil
}

func loadFileDescriptorSet(fdSet *nsproto.FileDescriptorSet, msgName string) (*schemaDescr, error) {
	// assuming file descriptors are topological sorted
	var dependencies []*desc.FileDescriptor
	var curfd *desc.FileDescriptor
	for i, fdb := range fdSet.Descriptors {
		fdp, err := decodeFileDescriptorProto(fdb)
		if err != nil {
			return nil, xerrors.Wrapf(err, "failed to decode file descriptor(%d) in version(%s)", i, fdSet.DeployId)
		}
		fd, err := desc.CreateFileDescriptor(fdp, dependencies...)
		if err != nil {
			return nil, xerrors.Wrapf(err, "failed to create file descriptor(%d) in version(%s)", i, fdSet.DeployId)
		}
		if !fd.IsProto3() {
			return nil, xerrors.Wrapf(errSyntaxNotProto3, "file descriptor(%s) is not proto3", fd.GetFullyQualifiedName())
		}
		curfd = fd
		dependencies = append(dependencies, curfd)
	}

	md := curfd.FindMessage(msgName)
	if md != nil {
		return newSchemaDescr(fdSet.DeployId, fdSet.PrevId, MessageDescriptor{md}), nil
	}
	return nil, xerrors.Wrapf(errInvalidSchemaOptions, "failed to find message (%s) in deployment(%s)", msgName, fdSet.DeployId)
}

// decodeFileDescriptorProto decodes the bytes of proto file descriptor.
func decodeFileDescriptorProto(fdb []byte) (*dpb.FileDescriptorProto, error) {
	fd := dpb.FileDescriptorProto{}

	if err := proto.Unmarshal(fdb, &fd); err != nil {
		return nil, err
	}
	return &fd, nil
}

// genDependencyDescriptors produces a topological sort of the dependency descriptors for the provided
// file descriptor, the result contains the input file descriptor as the last in the slice,
// the result contains indirect dependencies as well, dependencies in the return are distinct.
func genDependencyDescriptors(infd *desc.FileDescriptor) []*desc.FileDescriptor {
	var depfds []*desc.FileDescriptor
	dedup := make(map[string]struct{})

	for _, dep := range infd.GetDependencies() {
		depfs2 := genDependencyDescriptors(dep)
		for _, fd := range depfs2 {
			if _, ok := dedup[fd.GetFullyQualifiedName()]; !ok {
				dedup[fd.GetFullyQualifiedName()] = struct{}{}
				depfds = append(depfds, fd)
			}
		}
	}
	if _, ok := dedup[infd.GetFullyQualifiedName()]; !ok {
		depfds = append(depfds, infd)
		dedup[infd.GetFullyQualifiedName()] = struct{}{}
	}
	return depfds
}

// protoStringProvider provides proto contents from strings.
func protoStringProvider(source map[string]string) protoparse.FileAccessor {
	if len(source) == 0 {
		return nil
	}
	return func(filename string) (io.ReadCloser, error) {
		if contents, ok := source[filename]; ok {
			return ioutil.NopCloser(strings.NewReader(contents)), nil
		} else {
			return nil, os.ErrNotExist
		}
	}
}

func parseProto(protoFile string, accessor protoparse.FileAccessor, importPaths ...string) ([]*desc.FileDescriptor, error) {
	p := protoparse.Parser{ImportPaths: importPaths, Accessor: accessor, IncludeSourceCodeInfo: true}
	fds, err := p.ParseFiles(protoFile)
	if err != nil {
		return nil, xerrors.Wrapf(err, "failed to parse proto file: %s", protoFile)
	}
	if len(fds) == 0 {
		return nil, xerrors.Wrapf(errEmptyProtoFile, "proto file (%s) can not be parsed", protoFile)
	}
	if !fds[0].IsProto3() {
		return nil, xerrors.Wrapf(errSyntaxNotProto3, "proto file (%s) is not proto3", protoFile)
	}
	return genDependencyDescriptors(fds[0]), nil
}

func marshalFileDescriptors(fdList []*desc.FileDescriptor) ([][]byte, error) {
	var dlist [][]byte
	for _, fd := range fdList {
		fdbytes, err := proto.Marshal(fd.AsProto())
		if err != nil {
			return nil, xerrors.Wrapf(err, "failed to marshal file descriptor: %s", fd.GetFullyQualifiedName())
		}
		dlist = append(dlist, fdbytes)
	}
	return dlist, nil
}

// AppendSchemaOptions appends to a provided SchemaOptions with a new version of schema.
// The new version of schema is parsed out of the provided protoFile/msgName/contents.
// schemaOpt: the SchemaOptions to be appended to, if nil, a new SchemaOption is created.
// deployID: the version ID of the new schema.
// protoFile: name of the top level proto file.
// msgName: name of the top level proto message.
// contents: map of name to proto strings.
//          Except for the top level proto file, other imported proto files' key must be exactly the same
//          as how they are imported in the import statement:
//          E.g. if import.proto is imported as below
//          import "mainpkg/imported.proto";
//          Then the map key for improted.proto must be "mainpkg/imported.proto"
//          See src/dbnode/namesapce/kvadmin test for example.
func AppendSchemaOptions(schemaOpt *nsproto.SchemaOptions, protoFile, msgName string, contents map[string]string, deployID string) (*nsproto.SchemaOptions, error) {
	// Verify schema options
	schemaHist, err := LoadSchemaHistory(schemaOpt)
	if err != nil {
		return nil, xerrors.Wrap(err, "can not append to invalid schema history")
	}
	// Verify deploy ID
	if deployID == "" {
		return nil, errEmptyDeployID
	}
	if _, ok := schemaHist.Get(deployID); ok {
		return nil, errDuplicateDeployID
	}

	var prevID string
	if descr, ok := schemaHist.GetLatest(); ok {
		prevID = descr.DeployId()
	}

	out, err := parseProto(protoFile, protoStringProvider(contents))
	if err != nil {
		return nil, xerrors.Wrapf(err, "failed to parse schema from %v", protoFile)
	}

	dlist, err := marshalFileDescriptors(out)
	if err != nil {
		return nil, err
	}

	if schemaOpt == nil {
		schemaOpt = &nsproto.SchemaOptions{
			History:            &nsproto.SchemaHistory{},
			DefaultMessageName: msgName,
		}
	}
	schemaOpt.History.Versions = append(schemaOpt.History.Versions, &nsproto.FileDescriptorSet{DeployId: deployID, PrevId: prevID, Descriptors: dlist})
	schemaOpt.DefaultMessageName = msgName

	if _, err := LoadSchemaHistory(schemaOpt); err != nil {
		return nil, xerrors.Wrap(err, "new schema is not valid")
	}

	return schemaOpt, nil
}

func LoadSchemaRegistryFromFile(schemaReg SchemaRegistry, nsID ident.ID, deployID string, protoFile string, msgName string, importPath ...string) error {
	out, err := parseProto(protoFile, nil, importPath...)
	if err != nil {
		return xerrors.Wrapf(err, "failed to parse input proto file %v", protoFile)
	}

	dlist, err := marshalFileDescriptors(out)
	if err != nil {
		return err
	}

	schemaOpt := &nsproto.SchemaOptions{
		History: &nsproto.SchemaHistory{
			Versions: []*nsproto.FileDescriptorSet{{DeployId: deployID, Descriptors: dlist}},
		},
		DefaultMessageName: msgName,
	}
	schemaHis, err := LoadSchemaHistory(schemaOpt)
	if err != nil {
		return xerrors.Wrapf(err, "failed to load schema history from file: %v with msg: %v", protoFile, msgName)
	}
	err = schemaReg.SetSchemaHistory(nsID, schemaHis)
	if err != nil {
		return xerrors.Wrapf(err, "failed to update schema registry for %v", nsID.String())
	}
	return nil
}

func GenTestSchemaOptions(protoFile string, importPath ...string) *nsproto.SchemaOptions {
	out, _ := parseProto(protoFile, nil, importPath...)

	dlist, _ := marshalFileDescriptors(out)

	return &nsproto.SchemaOptions{
		History: &nsproto.SchemaHistory{
			Versions: []*nsproto.FileDescriptorSet{
				{DeployId: "first", Descriptors: dlist},
				{DeployId: "second", PrevId: "first", Descriptors: dlist},
				{DeployId: "third", PrevId: "second", Descriptors: dlist},
			},
		},
		DefaultMessageName: "mainpkg.TestMessage",
	}
}

func GetTestSchemaDescr(md *desc.MessageDescriptor) SchemaDescr {
	return &schemaDescr{md: MessageDescriptor{md}}
}

func GetTestSchemaDescrWithDeployID(md *desc.MessageDescriptor, deployID string) SchemaDescr {
	return &schemaDescr{md: MessageDescriptor{md}, deployId: deployID}
}
