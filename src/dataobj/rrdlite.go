package dataobj

type File struct {
	Key      interface{}
	Filename string
	Body     []byte
}

type RRDFileResp struct {
	Files []File
	Msg   string
}

type RRDFileQuery struct {
	Files []RRDFile
}

type RRDFile struct {
	Key      interface{}
	Filename string
}
