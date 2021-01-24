package template

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	texttemplate "text/template"
	"time"

	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/etcdserver/api/v3rpc/rpctypes"
)

var (
	ErrInitiliazed    = fmt.Errorf("already initialized")
	ErrNotInitialized = fmt.Errorf("not initialized")
)

func New(source, destination, command string, mode os.FileMode, backup bool, commandTimeout time.Duration) *Template {
	return &Template{
		Source:         source,
		Destination:    destination,
		Mode:           mode,
		Backup:         backup,
		Command:        command,
		CommandTimeout: commandTimeout,

		initialized: make(chan struct{}),
		set:         NewSet(),
		notify:      make(chan struct{}),
	}
}

type Template struct {
	Source         string
	Destination    string
	Mode           os.FileMode
	Backup         bool
	Command        string
	CommandTimeout time.Duration

	initializeOnce sync.Once
	initialized    chan struct{}
	client         *clientv3.Client
	set            *Set
	notify         chan struct{}
}

func (t *Template) Initialize(options ...Option) error {
	err := ErrInitiliazed
	t.initializeOnce.Do(func() {
		close(t.initialized)
		err = t.initialize(options...)
	})
	return err
}

func (t *Template) initialize(options ...Option) error {
	var opts Options
	opts.Apply(options...)

	endpoint := fmt.Sprintf("%s:%d", opts.Address, opts.Port)
	config := clientv3.Config{
		Endpoints: []string{endpoint},
	}
	client, err := clientv3.New(config)
	if err != nil {
		return err
	}
	t.client = client

	return nil
}

func (t *Template) isInitialized() bool {
	select {
	case <-t.initialized:
		return true
	default:
	}
	return false
}

func (t *Template) Process(ctx context.Context) error {
	if !t.isInitialized() {
		return ErrNotInitialized
	}

	// initial process
	if err := t.process(ctx); err != nil {
		return err
	}

	for _, key := range t.set.Keys() {
		go t.watch(ctx, key)
	}
	for {
		select {
		case <-t.notify:
			if err := t.process(ctx); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// unreachable
}

func (t *Template) process(ctx context.Context) error {
	data, err := t.render(ctx)
	if err != nil {
		return err
	}
	ok, err := sameFile(data, t.Destination)
	if err != nil {
		return err
	}
	if ok {
		// todo: print out the data (-dry mode)
		return nil
	}
	if err := atomicWriteFile(t.Destination, data, t.Mode); err != nil {
		return err
	}
	opctx, opcancel := context.WithTimeout(ctx, t.CommandTimeout)
	defer opcancel()
	_, err = t.runCommand(opctx, t.Command)
	if err != nil {
		return err
	}
	return nil
}

func (t *Template) render(ctx context.Context) ([]byte, error) {
	config := config{
		client: t.client,
		set:    t.set,
	}
	tmpl, err := texttemplate.New(filepath.Base(t.Source)).
		Funcs(Funcs(config)).ParseFiles(t.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template source")
	}
	buf := &bytes.Buffer{}
	if err := tmpl.Execute(buf, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (t *Template) runCommand(ctx context.Context, command string) ([]byte, error) {
	if command == "" {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	return cmd.CombinedOutput()
}

func (t *Template) watch(ctx context.Context, key string) {
	opts := []clientv3.OpOption{
		clientv3.WithProgressNotify(),
	}

createWatcher:
	// require leader
	// if the etcd endpoint is partitioned from the cluster, the watch stream will abort
	wch := t.client.Watch(
		clientv3.WithRequireLeader(ctx),
		key,
		opts...,
	)
	for {
		select {
		case wresp, ok := <-wch:
			if !ok {
				if ctx.Err() != nil {
					return
				}
				time.Sleep(250 * time.Millisecond)
				goto createWatcher
			}
			if err := wresp.Err(); err != nil {
				switch err {
				case rpctypes.ErrNoLeader:
					// todo: logging
					continue
				case rpctypes.ErrCompacted:
					// todo: logging
					continue
				default:
					return
				}
			}
			var changed bool
			for _, event := range wresp.Events {
				switch event.Type {
				// todo: what should we do if the key got deleted?
				case clientv3.EventTypeDelete:
					continue
				default:
				}
				key := string(event.Kv.Key)
				value := event.Kv.Value

				oldValue := t.set.Value(key)
				if !bytes.Equal(oldValue, value) {
					t.set.Put(key, value)
					changed = true
				}
			}
			if changed {
				t.notify <- struct{}{}
			}
		case <-t.client.Ctx().Done():
			return
		case <-ctx.Done():
			return
		}
	}
}

func sameFile(src []byte, dst string) (bool, error) {
	d, err := ioutil.ReadFile(dst)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !bytes.Equal(src, d) {
		return false, nil
	}
	return true, nil
}

func atomicWriteFile(name string, data []byte, perm os.FileMode) error {
	if name == "" {
		return fmt.Errorf("missing file name")
	}
	f, err := ioutil.TempFile(filepath.Dir(name), ".tmp-"+filepath.Base(name))
	if err != nil {
		return err
	}
	defer f.Close()
	if err := os.Chmod(f.Name(), perm); err != nil {
		return err
	}
	n, err := f.Write(data)
	if err == nil && n < len(data) {
		return io.ErrShortWrite
	}
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return os.Rename(f.Name(), name)
}
