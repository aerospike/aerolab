package ingest

func (i *Ingest) Download() error {
	err := i.DownloadS3()
	if err != nil {
		return err
	}
	return i.DownloadAsftp()
}

func (i *Ingest) DownloadS3() error {
	return nil // TODO
}

func (i *Ingest) DownloadAsftp() error {
	return nil // TODO
}
