// Package snowflake 提供全局唯一的 64 位主键生成器。
//
// 采用 Twitter Snowflake 变体：41 位毫秒时间戳 + 10 位工作节点 ID + 12 位序列号。
// 单节点每毫秒最多 4096 个 ID，整体是 64 位正整数且趋势递增。
//
// 为什么不用数据库自增：
//
//   - 多实例写入时自增需要额外的发号中心，分库分表后更难维护
//   - 自增 ID 会泄漏业务规模（外部按 ID 增速就能推算订单量）
//   - 主键在应用层生成，INSERT 之前就拿到了 ID，省掉一次 LastInsertId 回读
//   - 趋势递增，对 InnoDB 聚簇索引友好，不会像 UUID 那样造成页分裂
//
// 时钟回拨：底层库在回拨较小时自旋等待、回拨过大时直接返回错误，
// 不会在回拨窗口内生成重复 ID。
//
// 唯一性前提：**同一时刻不同实例的 nodeID 必须不同**。
// 单机单实例用自动推导即可；k8s 多 Pod / 多机部署必须显式配置 app.node_id。
package snowflake

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"sync"

	"github.com/bwmarrin/snowflake"
)

// MaxNodeID 工作节点 ID 上限（10 位，取值 0-1023）
const MaxNodeID = 1023

// AutoNodeID 传给 Setup 表示按主机名推导节点 ID
const AutoNodeID int64 = -1

// ErrNotInitialized 未调用 Setup 就取 ID
var ErrNotInitialized = errors.New("雪花 ID 生成器未初始化，请先调用 snowflake.Setup")

var (
	mu     sync.RWMutex
	node   *snowflake.Node
	nodeID int64 = AutoNodeID
)

// Setup 初始化生成器。nodeID 取值 0-1023，传 AutoNodeID 时按主机名推导。
//
// 可重复调用（测试里每个用例重建环境时会用到），后一次覆盖前一次。
func Setup(id int64) error {
	resolved, err := resolveNodeID(id)
	if err != nil {
		return err
	}
	n, err := snowflake.NewNode(resolved)
	if err != nil {
		return fmt.Errorf("初始化雪花 ID 生成器失败(node=%d): %w", resolved, err)
	}

	mu.Lock()
	node = n
	nodeID = resolved
	mu.Unlock()
	return nil
}

// NodeID 返回当前生效的节点 ID，未初始化时返回 AutoNodeID
func NodeID() int64 {
	mu.RLock()
	defer mu.RUnlock()
	return nodeID
}

// Next 生成一个全局唯一 ID，未初始化时返回 ErrNotInitialized
func Next() (int64, error) {
	mu.RLock()
	n := node
	mu.RUnlock()
	if n == nil {
		return 0, ErrNotInitialized
	}
	return n.Generate().Int64(), nil
}

// resolveNodeID 解析节点 ID。
//
// 自动模式按主机名做 FNV-1a 哈希取模：同一台机器结果稳定（重启不换号），
// 不同机器大概率不同。注意这是**概率性**的，集群部署请显式配置。
func resolveNodeID(id int64) (int64, error) {
	if id >= 0 {
		if id > MaxNodeID {
			return 0, fmt.Errorf("app.node_id 需在 0-%d 之间，当前 %d", MaxNodeID, id)
		}
		return id, nil
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		// 拿不到主机名就用 0：单实例场景没有影响，集群场景本就应该显式配置
		return 0, nil
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(host))
	return int64(h.Sum32() % (MaxNodeID + 1)), nil
}
