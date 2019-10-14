# Building aeromon requires access to the citrusleaf repo
# In the docker build process use is made of ssh-agent to retrieve keys
# Here we check that ssh-agent is running
if [ -z "$SSH_AUTH_SOCK" ]
then
    eval `ssh-agent -s`
    ssh-add
    export SSH_AUTH_SOCK	
    echo $SSH_AUTH_SOCK
    echo "ssh-agent started - not previously started"
    echo "you need a key that has access to citrusleaf in your .ssh folder"
    echo "if you get docker build errors this may be why - check the output"
    echo
fi

# Next we check that the keys can be used for github access
GITHUB_ACCESS_RESPONSE=`ssh  git@github.com 2>&1 | grep -i "successfully authenticated"`

if [ -z "${GITHUB_ACCESS_RESPONSE}" ]
then
	echo "Can't access github using credentials in .ssh - investigate"
	exit 1
fi
