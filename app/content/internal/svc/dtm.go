package svc

import (
	"database/sql"

	"github.com/dtm-labs/dtm/client/dtmcli"
	"github.com/dtm-labs/dtm/client/dtmgrpc"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

const barrierTableName = "dtm_barrier"

type PostCreateMsg interface {
	Add(action string, payload proto.Message)
	DoAndSubmitDB(queryPrepared string, fn func(*sql.Tx) error) error
}

type PostCreateMsgFactory interface {
	NewGID() string
	NewPostCreateMsg(gid string) PostCreateMsg
}

type DTMPostCreateMsgFactory struct {
	DtmServer string
	DB        *sql.DB
}

func (f DTMPostCreateMsgFactory) NewGID() string {
	return uuid.NewString()
}

func (f DTMPostCreateMsgFactory) NewPostCreateMsg(gid string) PostCreateMsg {
	return dtmPostCreateMsg{msg: dtmgrpc.NewMsgGrpc(f.DtmServer, gid), db: f.DB}
}

type dtmPostCreateMsg struct {
	msg *dtmgrpc.MsgGrpc
	db  *sql.DB
}

func (m dtmPostCreateMsg) Add(action string, payload proto.Message) {
	m.msg.Add(action, payload)
}

func (m dtmPostCreateMsg) DoAndSubmitDB(queryPrepared string, fn func(*sql.Tx) error) error {
	return m.msg.DoAndSubmitDB(queryPrepared, m.db, func(tx *sql.Tx) error {
		return fn(tx)
	})
}

func configureDTMBarrierTable() {
	dtmcli.SetBarrierTableName(barrierTableName)
}
