package rpcclient

import (
	"encoding/json"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/rpcclient"
)

type Descriptor struct {
	Desc      string      `json:"desc"`
	Active    *bool       `json:"active,omitempty"`
	Range     interface{} `json:"range,omitempty"`
	NextIndex *int        `json:"next_index,omitempty"`
	Timestamp interface{} `json:"timestamp"`
	Internal  *bool       `json:"internal,omitempty"`
	Label     *string     `json:"label,omitempty"`
}

// ImportDescriptorsCmd @see https://developer.bitcoin.org/reference/rpc/importdescriptors.html
type ImportDescriptorsCmd struct {
	Descriptors []Descriptor `json:""`
}

func NewImportDescriptorsCmd(descriptors []Descriptor) *ImportDescriptorsCmd {
	return &ImportDescriptorsCmd{
		Descriptors: descriptors,
	}
}

type ImportDescriptorsResultElement struct {
	Success  bool              `json:"success"`
	Warnings []string          `json:"warnings,omitempty"`
	Error    *btcjson.RPCError `json:"error,omitempty"`
}

type ImportDescriptorsResult []ImportDescriptorsResultElement

type FutureImportDescriptorsResult chan *rpcclient.Response

func (r FutureImportDescriptorsResult) Receive() (*ImportDescriptorsResult, error) {
	res, err := rpcclient.ReceiveFuture(r)
	if err != nil {
		return nil, err
	}

	var importDescriptors ImportDescriptorsResult
	err = json.Unmarshal(res, &importDescriptors)
	if err != nil {
		return nil, err
	}

	return &importDescriptors, nil
}

func ImportDescriptorsAsync(c *rpcclient.Client, descriptors []Descriptor) FutureImportDescriptorsResult {
	cmd := &ImportDescriptorsCmd{
		Descriptors: descriptors,
	}
	return c.SendCmd(cmd)
}

func ImportDescriptors(c *rpcclient.Client, descriptors []Descriptor) (*ImportDescriptorsResult, error) {
	return ImportDescriptorsAsync(c, descriptors).Receive()
}

func init() {
	flags := btcjson.UsageFlag(0)
	btcjson.MustRegisterCmd("importdescriptors", (*ImportDescriptorsCmd)(nil), flags)
}
