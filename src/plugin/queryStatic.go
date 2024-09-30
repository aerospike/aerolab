package plugin

import (
	"encoding/json"
	"os"

	"github.com/bestmethod/logger"
)

type staticResponse tableResponse

func (p *Plugin) handleQueryStatic(req *queryRequest, i int, remote string) (*staticResponse, error) {
	logger.Debug("Query start (type:static) (target:%d:%s) (refId:%s) (remote:%s)", i, req.Targets[i].Target, req.Targets[i].RefId, remote)
	defer logger.Debug("Query end (type:static) (target:%d:%s) (refId:%s) (remote:%s)", i, req.Targets[i].Target, req.Targets[i].RefId, remote)
	f, err := os.Open(req.Targets[i].Payload.Static.File)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data := make(map[string]interface{})
	err = json.NewDecoder(f).Decode(&data)
	if err != nil {
		return nil, err
	}
	response := &staticResponse{
		Type: "table",
	}
	var responseRows []interface{}
	if req.Targets[i].Payload.Static.Name != "" {
		req.Targets[i].Payload.Static.Names = append(req.Targets[i].Payload.Static.Names, req.Targets[i].Payload.Static.Name)
	}
	for _, dataName := range req.Targets[i].Payload.Static.Names {
		dataValue, ok := data[dataName]
		if !ok {
			dataValue = ""
		}
		nType := "string"
		switch dataValue.(type) {
		case int, int64, float64, int32, float32:
			nType = "number"
		}
		response.Columns = append(response.Columns, &tableColumn{
			Text: dataName,
			Type: nType,
		})
		responseRows = append(responseRows, dataValue)
	}
	response.Rows = append(response.Rows, responseRows)
	return response, nil
}
