package example.com;

import java.util.concurrent.atomic.AtomicInteger;

import com.aerospike.client.ScanCallback;
import com.aerospike.client.Key;
import com.aerospike.client.Record;

public class MyCallback implements ScanCallback {
        public AtomicInteger recordCount;
    
    
        @Override
        public void scanCallback(Key key, Record record) {
            // Scan callbacks must ensure thread safety when ScanAll() is used with
            // ScanPolicy concurrentNodes set to true (default).  In this case, parallel
            // node threads will be sending data to this callback.
            System.out.println(recordCount.incrementAndGet());
    
            /*
            synchronized (this) {
                int pid = Partition.getPartitionId(key.digest);
                console.info("PartId=" + pid + " Record=" + record);
            }
            */
        }
    }

