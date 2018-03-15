/*

  Copyright 2017 Loopring Project Ltd (Loopring Foundation).

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.

*/

package ordermanager

import (
	"github.com/Loopring/relay/dao"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/marketcap"
	"github.com/Loopring/relay/types"
	"math/big"
)

type forkProcessor struct {
	db dao.RdsService
	mc marketcap.MarketCapProvider
}

func newForkProcess(rds dao.RdsService, mc marketcap.MarketCapProvider) *forkProcessor {
	processor := &forkProcessor{}
	processor.db = rds
	processor.mc = mc

	return processor
}

//	todo(fuk): fork逻辑重构
// 1.从各个事件表中获取所有处于分叉块中的事件(fill,cancel,cutoff,cutoffPair)并按照blockNumber以及logIndex倒序
// 2.对每个事件进行回滚处理,并更新数据库,即便订单已经过期也要对其相关数据进行更新
// 3.删除所有分叉事件,或者对其进行标记
func (p *forkProcessor) fork(event *types.ForkedEvent) error {
	log.Debugf("order manager processing chain fork......")

	from := event.ForkBlock.Int64()
	to := event.DetectedBlock.Int64()

	if err := p.db.RollBackRingMined(from, to); err != nil {
		log.Errorf("order manager fork error:%s", err.Error())
	}
	if err := p.db.RollBackFill(from, to); err != nil {
		log.Errorf("order manager fork error:%s", err.Error())
	}
	//if err := p.db.RollBackCancel(from, to); err != nil {
	//	log.Errorf("order manager fork error:%s", err.Error())
	//}
	//if err := p.db.RollBackCutoff(from, to); err != nil {
	//	log.Errorf("order manager fork error:%s", err.Error())
	//}

	// todo(fuk): isOrderCutoff???
	orderList, err := p.db.GetOrdersWithBlockNumberRange(from, to)
	if err != nil {
		return err
	}

	forkBlockNumber := big.NewInt(from)
	for _, v := range orderList {
		state := &types.OrderState{}
		if err := v.ConvertUp(state); err != nil {
			log.Errorf("order manager fork error:%s", err.Error())
			continue
		}

		model, err := newOrderEntity(state, p.mc, forkBlockNumber)
		if err != nil {
			log.Errorf("order manager fork error:%s", err.Error())
			continue
		}

		model.ID = v.ID
		if err := p.db.Save(model); err != nil {
			log.Debugf("order manager fork error:%s", err.Error())
			continue
		}
	}

	return nil
}

type InnerForkEvent struct {
	Type        string
	BlockNumber int64
	LogIndex    int64
	Event       interface{}
}

type InnerForkEventList []InnerForkEvent

func (l InnerForkEventList) Len() int {
	return len(l)
}

func (l InnerForkEventList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l InnerForkEventList) Less(i, j int) bool {
	if l[i].BlockNumber == l[j].BlockNumber {
		return l[i].LogIndex > l[j].LogIndex
	} else {
		return l[i].BlockNumber > l[j].BlockNumber
	}
}
