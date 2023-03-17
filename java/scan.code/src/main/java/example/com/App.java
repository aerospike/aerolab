package example.com;

import java.util.concurrent.atomic.AtomicInteger;
import com.aerospike.client.AerospikeClient;
import com.aerospike.client.policy.ScanPolicy;



public class App
{
    public static void main( String[] args )
    {
        AerospikeClient client = new AerospikeClient("172.17.0.3", 3000);

        ScanPolicy policy = new ScanPolicy();
        policy.recordsPerSecond = 100;
        

        MyCallback myCallback = new MyCallback();
        myCallback.recordCount = new AtomicInteger();
        

        client.scanAll(policy, "test", "myset", myCallback);
        System.out.println(myCallback.recordCount.get());
        client.close();
    }

}

