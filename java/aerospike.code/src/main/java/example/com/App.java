package example.com;

import com.aerospike.client.AerospikeClient;

public class App 
{
    public static void main( String[] args )
    {
        AerospikeClient client = new AerospikeClient("172.17.0.2", 3000);

        // TODO code here

        client.close();
    }
}
