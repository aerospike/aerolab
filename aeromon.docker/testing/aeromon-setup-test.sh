SCRIPT_UNDER_TEST=aeromon-setup.sh

PATH=.:$PATH

if [ -z `which $SCRIPT_UNDER_TEST` ] 
then
	echo Script under test $SCRIPT_UNDER_TEST not found
	exit 1
fi

handle_results(){
	TEST_NAME="$1"
	EXPECTED_OUTPUT="$2"
	OUTPUT="$3"
	if [ "$OUTPUT" != "$EXPECTED_OUTPUT" ]
	then
		echo Test \'$TEST_NAME\' failed
		echo Expected output : $EXPECTED_OUTPUT
		echo Actual output : $OUTPUT
		exit 1
	else
		echo Test \'$TEST_NAME\' passed
	fi
}
	
TEST_1_NAME="No arguments"
TEST_1_OUTPUT=`${SCRIPT_UNDER_TEST}`
TEST_1_EXPECTED_OUTPUT="usage : ./${SCRIPT_UNDER_TEST} CLUSTER_NAME"

handle_results "$TEST_1_NAME" "$TEST_1_EXPECTED_OUTPUT" "$TEST_1_OUTPUT"

NO_SUCH_CLUSTER_NAME=no-such-cluster
TEST_2_NAME="Cluster does not exist"
TEST_2_OUTPUT=`${SCRIPT_UNDER_TEST} ${NO_SUCH_CLUSTER_NAME}`
TEST_2_EXPECTED_OUTPUT="No Aerospike cluster with name ${NO_SUCH_CLUSTER_NAME} found"

handle_results "$TEST_2_NAME" "$TEST_2_EXPECTED_OUTPUT" "$TEST_2_OUTPUT"

