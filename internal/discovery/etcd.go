package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/yedou37/dbd/internal/model"
)

const nodesPrefix = "/dbd/nodes/"

type Client struct {
	cli         *clientv3.Client
	leaseID     clientv3.LeaseID
	cancelRenew context.CancelFunc
	mu          sync.RWMutex
	lastNode    *model.NodeInfo
}

func New(endpoints []string) (*Client, error) {
	if len(endpoints) == 0 {
		return nil, nil
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 3 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	return &Client{cli: cli}, nil
}

func (c *Client) Register(ctx context.Context, node model.NodeInfo) error {
	if c == nil || c.cli == nil {
		return nil
	}

	lease, err := c.cli.Grant(ctx, 10)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(node)
	if err != nil {
		return err
	}

	if _, err := c.cli.Put(ctx, nodesPrefix+node.ID, string(payload), clientv3.WithLease(lease.ID)); err != nil {
		return err
	}

	renewCtx, cancel := context.WithCancel(context.Background())
	keepAlive, err := c.cli.KeepAlive(renewCtx, lease.ID)
	if err != nil {
		cancel()
		return err
	}

	c.leaseID = lease.ID
	c.cancelRenew = cancel
	c.setLastNode(node)

	go func() {
		for range keepAlive {
		}
	}()

	return nil
}

func (c *Client) Update(ctx context.Context, node model.NodeInfo) error {
	if c == nil || c.cli == nil {
		return nil
	}
	if c.leaseID == 0 {
		return c.Register(ctx, node)
	}

	payload, err := json.Marshal(node)
	if err != nil {
		return err
	}

	if _, err := c.cli.Put(ctx, nodesPrefix+node.ID, string(payload), clientv3.WithLease(c.leaseID)); err != nil {
		return err
	}

	c.setLastNode(node)
	return nil
}

func (c *Client) ListNodes(ctx context.Context) ([]model.NodeInfo, error) {
	if c == nil || c.cli == nil {
		return nil, nil
	}

	response, err := c.cli.Get(ctx, nodesPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	nodes := make([]model.NodeInfo, 0, len(response.Kvs))
	for _, kv := range response.Kvs {
		var node model.NodeInfo
		if err := json.Unmarshal(kv.Value, &node); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	slices.SortFunc(nodes, func(a, b model.NodeInfo) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	return nodes, nil
}

func (c *Client) FindLeader(ctx context.Context) (*model.NodeInfo, error) {
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if node.IsLeader {
			leader := node
			return &leader, nil
		}
	}

	return nil, errors.New("leader not found")
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	if c.cancelRenew != nil {
		c.cancelRenew()
	}
	if c.cli != nil {
		return c.cli.Close()
	}
	return nil
}

func (c *Client) setLastNode(node model.NodeInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copyNode := node
	c.lastNode = &copyNode
}
