package storage

import (
	"encoding/binary"
	"fmt"
	"os"
	"rdbms/internal/types"
)

const (
	btreeOrder = 32 // Maximum number of children per node
)

// BTreeNode represents a node in the B-tree
type BTreeNode struct {
	IsLeaf   bool
	Keys     []*types.Value
	Values  []int64 // Row positions in the data file
	Children []int64 // File positions of child nodes
}

// BTreeIndex represents a B-tree index
type BTreeIndex struct {
	rootPos   int64
	file      *os.File
	nodeSize  int
	freeList  []int64 // Positions of freed nodes
}

// NewBTreeIndex creates or opens a B-tree index
func NewBTreeIndex(indexPath string) (*BTreeIndex, error) {
	file, err := os.OpenFile(indexPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open index file: %w", err)
	}

	bt := &BTreeIndex{
		file:     file,
		nodeSize: 4096, // 4KB nodes
	}

	// Check if file is empty (new index)
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if stat.Size() == 0 {
		// Create root node
		root := &BTreeNode{
			IsLeaf:   true,
			Keys:     []*types.Value{},
			Values:   []int64{},
			Children: []int64{},
		}
		pos, err := bt.writeNode(root)
		if err != nil {
			return nil, err
		}
		bt.rootPos = pos
		bt.writeHeader()
	} else {
		// Read header
		bt.readHeader()
	}

	return bt, nil
}

func (bt *BTreeIndex) writeHeader() error {
	header := make([]byte, 8)
	binary.BigEndian.PutUint64(header, uint64(bt.rootPos))
	_, err := bt.file.WriteAt(header, 0)
	return err
}

func (bt *BTreeIndex) readHeader() error {
	header := make([]byte, 8)
	_, err := bt.file.ReadAt(header, 0)
	if err != nil {
		return err
	}
	bt.rootPos = int64(binary.BigEndian.Uint64(header))
	return nil
}

func (bt *BTreeIndex) writeNode(node *BTreeNode) (int64, error) {
	var pos int64

	// Try to reuse a freed position
	if len(bt.freeList) > 0 {
		pos = bt.freeList[len(bt.freeList)-1]
		bt.freeList = bt.freeList[:len(bt.freeList)-1]
	} else {
		stat, err := bt.file.Stat()
		if err != nil {
			return 0, err
		}
		pos = stat.Size()
	}

	buf := make([]byte, bt.nodeSize)
	offset := 0

	// Write isLeaf flag
	if node.IsLeaf {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}
	offset++

	// Write number of keys
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(node.Keys)))
	offset += 2

	// Write keys and values
	for i, key := range node.Keys {
		keyData, err := key.Serialize()
		if err != nil {
			return 0, err
		}
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(keyData)))
		offset += 2
		copy(buf[offset:], keyData)
		offset += len(keyData)

		binary.BigEndian.PutUint64(buf[offset:], uint64(node.Values[i]))
		offset += 8
	}

	// Write children (for non-leaf nodes)
	if !node.IsLeaf {
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(node.Children)))
		offset += 2
		for _, child := range node.Children {
			binary.BigEndian.PutUint64(buf[offset:], uint64(child))
			offset += 8
		}
	}

	_, err := bt.file.WriteAt(buf[:offset], pos)
	return pos, err
}

func (bt *BTreeIndex) readNode(pos int64) (*BTreeNode, error) {
	buf := make([]byte, bt.nodeSize)
	_, err := bt.file.ReadAt(buf, pos)
	if err != nil {
		return nil, err
	}

	node := &BTreeNode{}
	offset := 0

	// Read isLeaf flag
	node.IsLeaf = buf[offset] == 1
	offset++

	// Read number of keys
	numKeys := int(binary.BigEndian.Uint16(buf[offset:]))
	offset += 2

	// Read keys and values
	node.Keys = make([]*types.Value, numKeys)
	node.Values = make([]int64, numKeys)
	for i := 0; i < numKeys; i++ {
		keyLen := int(binary.BigEndian.Uint16(buf[offset:]))
		offset += 2
		keyData := buf[offset : offset+keyLen]
		offset += keyLen

		// For simplicity, assume INTEGER type - in production, store type info
		key, err := types.DeserializeValue(types.TypeInteger, keyData)
		if err != nil {
			return nil, err
		}
		node.Keys[i] = key

		node.Values[i] = int64(binary.BigEndian.Uint64(buf[offset:]))
		offset += 8
	}

	// Read children (for non-leaf nodes)
	if !node.IsLeaf {
		numChildren := int(binary.BigEndian.Uint16(buf[offset:]))
		offset += 2
		node.Children = make([]int64, numChildren)
		for i := 0; i < numChildren; i++ {
			node.Children[i] = int64(binary.BigEndian.Uint64(buf[offset:]))
			offset += 8
		}
	}

	return node, nil
}

// Insert inserts a key-value pair into the B-tree
func (bt *BTreeIndex) Insert(key *types.Value, rowPos int64) error {
	root, err := bt.readNode(bt.rootPos)
	if err != nil {
		return err
	}

	if len(root.Keys) >= btreeOrder-1 {
		// Root is full, split it
		newRoot := &BTreeNode{
			IsLeaf:   false,
			Keys:     []*types.Value{},
			Values:   []int64{},
			Children: []int64{bt.rootPos},
		}
		newRootPos, err := bt.writeNode(newRoot)
		if err != nil {
			return err
		}
		bt.rootPos = newRootPos
		bt.splitChild(newRootPos, 0)
		bt.writeHeader()
		root, err = bt.readNode(bt.rootPos)
		if err != nil {
			return err
		}
	}

	return bt.insertNonFull(bt.rootPos, key, rowPos)
}

func (bt *BTreeIndex) insertNonFull(nodePos int64, key *types.Value, rowPos int64) error {
	node, err := bt.readNode(nodePos)
	if err != nil {
		return err
	}

	if node.IsLeaf {
		// Insert into leaf
		idx := bt.findInsertIndex(node.Keys, key)
		node.Keys = append(node.Keys, nil)
		node.Values = append(node.Values, 0)
		copy(node.Keys[idx+1:], node.Keys[idx:])
		copy(node.Values[idx+1:], node.Values[idx:])
		node.Keys[idx] = key
		node.Values[idx] = rowPos
		_, err := bt.writeNode(node)
		return err
	}

	// Find child to insert into
	idx := bt.findInsertIndex(node.Keys, key)
	childPos := node.Children[idx]
	child, err := bt.readNode(childPos)
	if err != nil {
		return err
	}

	if len(child.Keys) >= btreeOrder-1 {
		// Child is full, split it
		bt.splitChild(nodePos, idx)
		// Re-read node after split
		node, err = bt.readNode(nodePos)
		if err != nil {
			return err
		}
		// Determine which child to insert into
		if cmp, _ := key.Compare(node.Keys[idx]); cmp > 0 {
			idx++
		}
		childPos = node.Children[idx]
	}

	return bt.insertNonFull(childPos, key, rowPos)
}

func (bt *BTreeIndex) splitChild(parentPos int64, idx int) error {
	parent, err := bt.readNode(parentPos)
	if err != nil {
		return err
	}

	child, err := bt.readNode(parent.Children[idx])
	if err != nil {
		return err
	}

	// Create new node for right half
	newNode := &BTreeNode{
		IsLeaf: child.IsLeaf,
		Keys:    make([]*types.Value, len(child.Keys)/2),
		Values:  make([]int64, len(child.Values)/2),
	}
	if !child.IsLeaf {
		newNode.Children = make([]int64, len(child.Children)/2)
	}

	mid := len(child.Keys) / 2
	copy(newNode.Keys, child.Keys[mid:])
	copy(newNode.Values, child.Values[mid:])
	if !child.IsLeaf {
		copy(newNode.Children, child.Children[mid:])
	}

	// Truncate child
	child.Keys = child.Keys[:mid]
	child.Values = child.Values[:mid]
	if !child.IsLeaf {
		child.Children = child.Children[:mid]
	}

	// Write nodes
	newNodePos, err := bt.writeNode(newNode)
	if err != nil {
		return err
	}
	_, err = bt.writeNode(child)
	if err != nil {
		return err
	}

	// Insert middle key into parent
	parent.Keys = append(parent.Keys, nil)
	parent.Values = append(parent.Values, 0)
	parent.Children = append(parent.Children, 0)
	copy(parent.Keys[idx+1:], parent.Keys[idx:])
	copy(parent.Values[idx+1:], parent.Values[idx:])
	copy(parent.Children[idx+2:], parent.Children[idx+1:])
	parent.Keys[idx] = newNode.Keys[0]
	parent.Values[idx] = newNode.Values[0]
	parent.Children[idx+1] = newNodePos

	_, err = bt.writeNode(parent)
	return err
}

func (bt *BTreeIndex) findInsertIndex(keys []*types.Value, key *types.Value) int {
	for i, k := range keys {
		if cmp, _ := key.Compare(k); cmp < 0 {
			return i
		}
	}
	return len(keys)
}

// Search searches for a key in the B-tree
func (bt *BTreeIndex) Search(key *types.Value) (int64, bool) {
	return bt.searchNode(bt.rootPos, key)
}

func (bt *BTreeIndex) searchNode(nodePos int64, key *types.Value) (int64, bool) {
	node, err := bt.readNode(nodePos)
	if err != nil {
		return 0, false
	}

	idx := 0
	for idx < len(node.Keys) {
		cmp, _ := key.Compare(node.Keys[idx])
		if cmp == 0 {
			return node.Values[idx], true
		}
		if cmp < 0 {
			break
		}
		idx++
	}

	if node.IsLeaf {
		return 0, false
	}

	return bt.searchNode(node.Children[idx], key)
}

// RangeSearch searches for all keys in a range
func (bt *BTreeIndex) RangeSearch(min, max *types.Value) []int64 {
	results := []int64{}
	bt.rangeSearchNode(bt.rootPos, min, max, &results)
	return results
}

func (bt *BTreeIndex) rangeSearchNode(nodePos int64, min, max *types.Value, results *[]int64) {
	node, err := bt.readNode(nodePos)
	if err != nil {
		return
	}

	idx := 0
	for idx < len(node.Keys) {
		if min != nil {
			cmp, _ := node.Keys[idx].Compare(min)
			if cmp < 0 {
				idx++
				continue
			}
		}
		if max != nil {
			cmp, _ := node.Keys[idx].Compare(max)
			if cmp > 0 {
				break
			}
		}

		if !node.IsLeaf {
			bt.rangeSearchNode(node.Children[idx], min, max, results)
		}

		*results = append(*results, node.Values[idx])
		idx++
	}

	if !node.IsLeaf && idx < len(node.Children) {
		bt.rangeSearchNode(node.Children[idx], min, max, results)
	}
}

// Close closes the index file
func (bt *BTreeIndex) Close() error {
	return bt.file.Close()
}
