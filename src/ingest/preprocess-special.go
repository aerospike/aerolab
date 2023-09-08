package ingest

// special file handler type
type specialFile struct{}

// open the file and work out if it's a special type. preProcessNotAerospike and preProcessNotSpecial should be returned if needed
// if special type, also work out clusterName and nodeID if needed before continuing (for one-node-per-file formats or one-cluster-per-file formats)
func (i *Ingest) preProcessOpenSpecialFile(fn string) (*specialFile, error) {
	return nil, errPreProcessNotSpecial
}

// close the file handle of file being read
func (s *specialFile) close() {}

// read the next aerospike line and return true, or if EOF reached (or error) return false
func (s *specialFile) scan() bool {
	return false
}

// return the clusterName for this log line
func (s *specialFile) cluster() string {
	return ""
}

// return the nodeID for this log line
func (s *specialFile) nodeID() string {
	return ""
}

// return the aerospike log line
func (s *specialFile) line() string {
	return ""
}

// this is set if scan() had an error and is due to return false
func (s *specialFile) err() error {
	return nil
}
