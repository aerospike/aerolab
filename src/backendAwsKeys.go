package main

import (
	"fmt"
	"os"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func (d *backendAws) GetKeyPath(clusterName string) (keyPath string, err error) {
	_, p, e := d.getKey(clusterName)
	return p, e
}

// get KeyPair
func (d *backendAws) getKey(clusterName string) (keyName string, keyPath string, err error) {
	keyName = fmt.Sprintf("aerolab-%s_%s", clusterName, a.opts.Config.Backend.Region)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	// check keyName exists, if not, error
	filter := ec2.DescribeKeyPairsInput{}
	filter.KeyNames = []*string{&keyName}
	keys, err := d.ec2svc.DescribeKeyPairs(&filter)
	if err != nil {
		err = fmt.Errorf("could not DescribeKeypairs: %s", err)
	}
	if len(keys.KeyPairs) != 1 {
		err = fmt.Errorf("key pair does not exist in AWS")
	}
	// check keypath exists, if not, error
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		err = fmt.Errorf("key does not exist in given location: %s", keyPath)
		return keyName, keyPath, err
	}
	return
}

// get KeyPair
func (d *backendAws) makeKey(clusterName string) (keyName string, keyPath string, err error) {
	keyName = fmt.Sprintf("aerolab-%s_%s", clusterName, a.opts.Config.Backend.Region)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	_, _, err = d.getKey(clusterName)
	if err == nil {
		return
	}
	// check keypath exists, if not, make
	if _, err := os.Stat(string(a.opts.Config.Backend.SshKeyPath)); os.IsNotExist(err) {
		os.MkdirAll(string(a.opts.Config.Backend.SshKeyPath), 0700)
	}
	// generate keypair
	filter := ec2.CreateKeyPairInput{}
	filter.DryRun = aws.Bool(false)
	filter.KeyName = &keyName
	out, err := d.ec2svc.CreateKeyPair(&filter)
	if err != nil {
		err = fmt.Errorf("could not generate keypair: %s", err)
		return
	}
	err = os.WriteFile(keyPath, []byte(*out.KeyMaterial), 0600)
	keyName = fmt.Sprintf("aerolab-%s_%s", clusterName, a.opts.Config.Backend.Region)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	return
}

// get KeyPair
func (d *backendAws) killKey(clusterName string) (keyName string, keyPath string, err error) {
	keyName = fmt.Sprintf("aerolab-%s_%s", clusterName, a.opts.Config.Backend.Region)
	keyPath = path.Join(string(a.opts.Config.Backend.SshKeyPath), keyName)
	os.Remove(keyPath)
	filter := ec2.DeleteKeyPairInput{}
	filter.DryRun = aws.Bool(false)
	filter.KeyName = &keyName
	d.ec2svc.DeleteKeyPair(&filter)
	return
}
