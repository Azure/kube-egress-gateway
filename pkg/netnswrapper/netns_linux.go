/*
MIT License

Copyright (c) Microsoft Corporation.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE
*/
package netnswrapper

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"sync"

	"github.com/containernetworking/plugins/pkg/ns"
	"golang.org/x/sys/unix"
)

// Some of the following functions are migrated from
// https://github.com/containernetworking/plugins/blob/master/pkg/testutils/netns_linux.go
// https://github.com/containerd/containerd/blob/main/pkg/netns/netns_linux.go
const (
	nsBaseDir = "/var/run/netns"
)

type Interface interface {
	// NewNS creates a new named network namespace
	NewNS(nsName string) (ns.NetNS, error)
	// GetNS gets a named network namespace
	GetNS(nsName string) (ns.NetNS, error)
	// UnmountNS deletes a named network namespace
	UnmountNS(nsName string) error
	// ListNS lists all network namespaces
	ListNS() ([]string, error)
}

type netns struct{}

func NewNetNS() Interface {
	return &netns{}
}

func (*netns) NewNS(nsName string) (ns.NetNS, error) {
	// Create the directory for mounting network namespaces
	// This needs to be a shared mountpoint in case it is mounted in to
	// other namespaces (containers)
	err := os.MkdirAll(nsBaseDir, 0755)
	if err != nil {
		return nil, err
	}

	// create an empty file at the mount point and fail if it already exists
	nsPath := path.Join(nsBaseDir, nsName)
	mountPointFd, err := os.OpenFile(nsPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return nil, err
	}
	mountPointFd.Close()

	// Ensure the mount point is cleaned up on errors
	defer func() {
		if err != nil {
			os.RemoveAll(nsPath)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(1)

	// do namespace work in a dedicated goroutine, so that we can safely
	// Lock/Unlock OSThread without upsetting the lock/unlock state of
	// the caller of this function
	go (func() {
		defer wg.Done()
		runtime.LockOSThread()
		// Don't unlock. By not unlocking, golang will kill the OS thread when the
		// goroutine is done (for go1.10+)

		var origNS ns.NetNS
		origNS, err = ns.GetNS(getCurrentThreadNetNSPath())
		if err != nil {
			return
		}
		defer origNS.Close()

		// create a new netns on the current thread
		err = unix.Unshare(unix.CLONE_NEWNET)
		if err != nil {
			return
		}

		// Put this thread back to the orig ns, since it might get reused (pre go1.10)
		defer func() { _ = origNS.Set() }()

		// bind mount the netns from the current thread (from /proc) onto the
		// mount point. This causes the namespace to persist, even when there
		// are no threads in the ns.
		err = unix.Mount(getCurrentThreadNetNSPath(), nsPath, "none", unix.MS_BIND, "")
		if err != nil {
			err = fmt.Errorf("failed to bind mount ns at %s: %v", nsPath, err)
		}
	})()
	wg.Wait()

	if err != nil {
		return nil, fmt.Errorf("failed to create namespace: %v", err)
	}

	return ns.GetNS(nsPath)
}

func (*netns) GetNS(nsName string) (ns.NetNS, error) {
	nsPath := path.Join(nsBaseDir, nsName)
	return ns.GetNS(nsPath)
}

func (*netns) UnmountNS(nsName string) error {
	nsPath := path.Join(nsBaseDir, nsName)
	if err := unix.Unmount(nsPath, unix.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount NS: at %s: %v", nsPath, err)
	}

	if err := os.Remove(nsPath); err != nil {
		return fmt.Errorf("failed to remove ns path %s: %v", nsPath, err)
	}

	return nil
}

func (*netns) ListNS() ([]string, error) {
	dir, err := os.Open(nsBaseDir)
	if err != nil {
		return nil, err
	}
	files, err := dir.Readdir(0)
	if err != nil {
		return nil, err
	}
	var nsList []string
	for _, f := range files {
		if !f.IsDir() {
			nsList = append(nsList, f.Name())
		}
	}
	return nsList, nil
}

// getCurrentThreadNetNSPath copied from pkg/ns
func getCurrentThreadNetNSPath() string {
	// /proc/self/ns/net returns the namespace of the main thread, not
	// of whatever thread this goroutine is running on.  Make sure we
	// use the thread's net namespace since the thread is switching around
	return fmt.Sprintf("/proc/%d/task/%d/ns/net", os.Getpid(), unix.Gettid())
}
