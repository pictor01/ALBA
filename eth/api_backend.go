// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package eth

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/pictor01/ALBA"
	"github.com/pictor01/ALBA/accounts"
	"github.com/pictor01/ALBA/common"
	"github.com/pictor01/ALBA/consensus"
	"github.com/pictor01/ALBA/core"
	"github.com/pictor01/ALBA/core/bloombits"
	"github.com/pictor01/ALBA/core/rawdb"
	"github.com/pictor01/ALBA/core/state"
	"github.com/pictor01/ALBA/core/types"
	"github.com/pictor01/ALBA/core/vm"
	"github.com/pictor01/ALBA/alba/gasprice"
	"github.com/pictor01/ALBA/albadb"
	"github.com/pictor01/ALBA/event"
	"github.com/pictor01/ALBA/miner"
	"github.com/pictor01/ALBA/params"
	"github.com/pictor01/ALBA/rpc"
)

// AlbaAPIBackend implements albaapi.Backend for full nodes
type AlbaAPIBackend struct {
	extRPCEnabled       bool
	allowUnprotectedTxs bool
	alba                *Alba
	gpo                 *gasprice.Oracle
}

// ChainConfig returns the active chain configuration.
func (b *AlbaAPIBackend) ChainConfig() *params.ChainConfig {
	return b.alba.blockchain.Config()
}

func (b *AlbaAPIBackend) CurrentBlock() *types.Block {
	return b.alba.blockchain.CurrentBlock()
}

func (b *AlbaAPIBackend) SetHead(number uint64) {
	b.alba.handler.downloader.Cancel()
	b.alba.blockchain.SetHead(number)
}

func (b *AlbaAPIBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.alba.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.alba.blockchain.CurrentBlock().Header(), nil
	}
	return b.eth.blockchain.GetHeaderByNumber(uint64(number)), nil
}

func (b *AlbaAPIBackend) HeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.HeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.alba.blockchain.GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.eth.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		return header, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *AlbaAPIBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return b.alba.blockchain.GetHeaderByHash(hash), nil
}

func (b *AlbaAPIBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.alba.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.alba.blockchain.CurrentBlock(), nil
	}
	return b.alba.blockchain.GetBlockByNumber(uint64(number)), nil
}

func (b *AlbaAPIBackend) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return b.alba.blockchain.GetBlockByHash(hash), nil
}

func (b *AlbaAPIBackend) BlockByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Block, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.BlockByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.eth.blockchain.GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.alba.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		block := b.alba.blockchain.GetBlock(hash, header.Number.Uint64())
		if block == nil {
			return nil, errors.New("header found, but block body is missing")
		}
		return block, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *AlbaAPIBackend) PendingBlockAndReceipts() (*types.Block, types.Receipts) {
	return b.alba.miner.PendingBlockAndReceipts()
}

func (b *AlbaAPIBackend) StateAndHeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Pending state is only known by the miner
	if number == rpc.PendingBlockNumber {
		block, state := b.alba.miner.Pending()
		return state, block.Header(), nil
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, errors.New("header not found")
	}
	stateDb, err := b.alba.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *EthAPIBackend) StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.StateAndHeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header, err := b.HeaderByHash(ctx, hash)
		if err != nil {
			return nil, nil, err
		}
		if header == nil {
			return nil, nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.alba.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, nil, errors.New("hash is not currently canonical")
		}
		stateDb, err := b.alba.BlockChain().StateAt(header.Root)
		return stateDb, header, err
	}
	return nil, nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *AlbaAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	return b.alba.blockchain.GetReceiptsByHash(hash), nil
}

func (b *AlbaAPIBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	db := b.alba.ChainDb()
	number := rawdb.ReadHeaderNumber(db, hash)
	if number == nil {
		return nil, errors.New("failed to get block number from hash")
	}
	logs := rawdb.ReadLogs(db, hash, *number, b.alba.blockchain.Config())
	if logs == nil {
		return nil, errors.New("failed to get logs for block")
	}
	return logs, nil
}

func (b *AlbaAPIBackend) GetTd(ctx context.Context, hash common.Hash) *big.Int {
	if header := b.alba.blockchain.GetHeaderByHash(hash); header != nil {
		return b.alba.blockchain.GetTd(hash, header.Number.Uint64())
	}
	return nil
}

func (b *AlbaAPIBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmConfig *vm.Config) (*vm.EVM, func() error, error) {
	vmError := func() error { return nil }
	if vmConfig == nil {
		vmConfig = b.alba.blockchain.GetVMConfig()
	}
	txContext := core.NewEVMTxContext(msg)
	context := core.NewEVMBlockContext(header, b.eth.BlockChain(), nil)
	return vm.NewEVM(context, txContext, state, b.eth.blockchain.Config(), *vmConfig), vmError, nil
}

func (b *AlbaAPIBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.alba.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *AlbaAPIBackend) SubscribePendingLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.alba.miner.SubscribePendingLogs(ch)
}

func (b *AlbaAPIBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return b.alba.BlockChain().SubscribeChainEvent(ch)
}

func (b *AlbaAPIBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return b.alba.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *AlbaAPIBackend) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return b.alba.BlockChain().SubscribeChainSideEvent(ch)
}

func (b *AlbaAPIBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.alba.BlockChain().SubscribeLogsEvent(ch)
}

func (b *AlbaAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.alba.txPool.AddLocal(signedTx)
}

func (b *AlbaAPIBackend) GetPoolTransactions() (types.Transactions, error) {
	pending := b.alba.txPool.Pending(false)
	var txs types.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *AlbaAPIBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.alba.txPool.Get(hash)
}

func (b *AlbaAPIBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	tx, blockHash, blockNumber, index := rawdb.ReadTransaction(b.alba.ChainDb(), txHash)
	return tx, blockHash, blockNumber, index, nil
}

func (b *AlbaAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.alba.txPool.Nonce(addr), nil
}

func (b *AlbaAPIBackend) Stats() (pending int, queued int) {
	return b.alba.txPool.Stats()
}

func (b *AlbaAPIBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.alba.TxPool().Content()
}

func (b *AlbaAPIBackend) TxPoolContentFrom(addr common.Address) (types.Transactions, types.Transactions) {
	return b.alba.TxPool().ContentFrom(addr)
}

func (b *AlbaAPIBackend) TxPool() *core.TxPool {
	return b.alba.TxPool()
}

func (b *AlbaAPIBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return b.alba.TxPool().SubscribeNewTxsEvent(ch)
}

func (b *AlbaAPIBackend) SyncProgress() alba.SyncProgress {
	return b.alba.Downloader().Progress()
}

func (b *AlbaAPIBackend) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestTipCap(ctx)
}

func (b *AlbaAPIBackend) FeeHistory(ctx context.Context, blockCount int, lastBlock rpc.BlockNumber, rewardPercentiles []float64) (firstBlock *big.Int, reward [][]*big.Int, baseFee []*big.Int, gasUsedRatio []float64, err error) {
	return b.gpo.FeeHistory(ctx, blockCount, lastBlock, rewardPercentiles)
}

func (b *AlbaAPIBackend) ChainDb() albadb.Database {
	return b.alba.ChainDb()
}

func (b *AlbaAPIBackend) EventMux() *event.TypeMux {
	return b.alba.EventMux()
}

func (b *AlbaAPIBackend) AccountManager() *accounts.Manager {
	return b.alba.AccountManager()
}

func (b *AlbaAPIBackend) ExtRPCEnabled() bool {
	return b.extRPCEnabled
}

func (b *AlbaAPIBackend) UnprotectedAllowed() bool {
	return b.allowUnprotectedTxs
}

func (b *AlbaAPIBackend) RPCGasCap() uint64 {
	return b.alba.config.RPCGasCap
}

func (b *AlbaAPIBackend) RPCEVMTimeout() time.Duration {
	return b.alba.config.RPCEVMTimeout
}

func (b *AlbaAPIBackend) RPCTxFeeCap() float64 {
	return b.Alba.config.RPCTxFeeCap
}

func (b *AlbaAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.eth.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

func (b *AlbaAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.eth.bloomRequests)
	}
}

func (b *AlbaAPIBackend) Engine() consensus.Engine {
	return b.alba.engine
}

func (b *AlbaAPIBackend) CurrentHeader() *types.Header {
	return b.alba.blockchain.CurrentHeader()
}

func (b *AlbaAPIBackend) Miner() *miner.Miner {
	return b.alba.Miner()
}

func (b *AlbaAPIBackend) StartMining(threads int) error {
	return b.alba.StartMining(threads)
}

func (b *AlbaAPIBackend) StateAtBlock(ctx context.Context, block *types.Block, reexec uint64, base *state.StateDB, checkLive, preferDisk bool) (*state.StateDB, error) {
	return b.alba.StateAtBlock(block, reexec, base, checkLive, preferDisk)
}

func (b *AlbaAPIBackend) StateAtTransaction(ctx context.Context, block *types.Block, txIndex int, reexec uint64) (core.Message, vm.BlockContext, *state.StateDB, error) {
	return b.alba.stateAtTransaction(block, txIndex, reexec)
}
