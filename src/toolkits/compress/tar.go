package compress

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path"
)

func TarGzWrite(recPath string, tw *tar.Writer, fi os.FileInfo) error {
	fr, err := os.Open(recPath)
	if err != nil {
		return err
	}
	defer fr.Close()

	h := new(tar.Header)
	h.Name = recPath
	h.Size = fi.Size()
	h.Mode = int64(fi.Mode())
	h.ModTime = fi.ModTime()

	err = tw.WriteHeader(h)
	if err != nil {
		return err
	}

	_, err = io.Copy(tw, fr)
	return err
}

func IterDirectory(dirPath string, tw *tar.Writer) error {
	dir, err := os.Open(dirPath)
	if err != nil {
		return err
	}
	defer dir.Close()

	fis, err := dir.Readdir(0)
	if err != nil {
		return err
	}
	for _, fi := range fis {
		curPath := dirPath + "/" + fi.Name()
		if fi.IsDir() {
			//TarGzWrite( curPath, tw, fi )
			err = IterDirectory(curPath, tw)
			if err != nil {
				return err
			}
		} else {
			//fmt.Printf("adding... %s\n", curPath)
			err = TarGzWrite(curPath, tw, fi)
			if err != nil {
				return err
			}
		}
	}
	return err
}

func TarGz(outFilePath string, inPath string) error {
	// file write
	fw, err := os.Create(outFilePath)
	if err != nil {
		return err
	}
	defer fw.Close()

	// gzip write
	gw := gzip.NewWriter(fw)
	defer gw.Close()

	// tar write
	tw := tar.NewWriter(gw)
	defer tw.Close()

	err = IterDirectory(inPath, tw)
	return err

}

func UnTarGz(srcFilePath string, destDirPath string) error {
	// Create destination directory
	os.Mkdir(destDirPath, os.ModePerm)

	fr, err := os.Open(srcFilePath)
	if err != nil {
		return err
	}
	defer fr.Close()

	// Gzip reader
	gr, err := gzip.NewReader(fr)
	if err != nil {
		return err
	}

	// Tar reader
	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// End of tar archive
			break
		}

		// Check if it is diretory or file
		if hdr.Typeflag != tar.TypeDir {
			// Get files from archive
			// Create diretory before create file
			os.MkdirAll(destDirPath+"/"+path.Dir(hdr.Name), os.ModePerm)
			// Write data to file
			fw, err := os.Create(destDirPath + "/" + hdr.Name)
			if err != nil {
				return err
			}
			_, err = io.Copy(fw, tr)
			if err != nil {
				return err
			}
		}
	}
	return err
}
