package ingest

// RetainPutBatcherHold increments the shared putBatcher reference count.
// The live listener must call this before submitting rows (typically
// once when Serve starts). Pairs with ReleasePutBatcherHold.
func (i *Ingest) RetainPutBatcherHold() {
	i.putBatcherHolds.Add(1)
}

// ReleasePutBatcherHold decrements the putBatcher reference count and
// closes the batcher when it reaches zero. finalizeInit seeds the
// counter to one for the batch pipeline; Close and live shutdown each
// release one hold.
func (i *Ingest) ReleasePutBatcherHold() {
	for {
		cur := i.putBatcherHolds.Load()
		if cur <= 0 {
			return
		}
		if i.putBatcherHolds.CompareAndSwap(cur, cur-1) {
			if cur-1 == 0 && i.putBatcher != nil {
				i.putBatcher.close()
				i.putBatcher = nil
			}
			return
		}
	}
}
