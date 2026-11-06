package httpapi

// «Обновление данных» (администратор): результат последней загрузки каждого справочника хранится в
// data_update_results, чтобы «что изменилось» не пропадало после перезагрузки страницы.

import (
	"encoding/json"
	"log"
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/antidumping"
)

// saveDataUpdate запоминает результат загрузки. Ошибка не мешает загрузке: данные уже сохранены.
func (s *Server) saveDataUpdate(c *gin.Context, dataset string, result any) {
	data, err := json.Marshal(result)
	if err == nil {
		err = s.actions.SaveDataUpdate(c.Request.Context(), dataset, currentUser(c).ID, data)
	}
	if err != nil {
		log.Printf("data updates: save %s: %v", dataset, err)
	}
}

func (s *Server) listDataUpdates(c *gin.Context) {
	rows, err := s.actions.DataUpdates(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	out := gin.H{}
	for _, row := range rows {
		out[row.Dataset] = gin.H{"result": row.Result, "updatedAt": row.UpdatedAt, "updatedByName": row.UpdatedByName}
	}
	c.JSON(http.StatusOK, out)
}

type measureRef struct {
	Number    string `json:"number"`
	ShortName string `json:"shortName"`
}

// measureChanges — что поменялось в реестре ЕЭК при обновлении: новые, снятые и изменённые меры.
type measureChanges struct {
	MeasureCount int          `json:"measureCount"`
	Added        []measureRef `json:"added"`
	Removed      []measureRef `json:"removed"`
	Changed      []measureRef `json:"changed"`
}

func diffMeasures(before, after []antidumping.Measure) measureChanges {
	old := make(map[string]antidumping.Measure, len(before))
	for _, measure := range before {
		old[measure.Number] = measure
	}
	changes := measureChanges{MeasureCount: len(after), Added: []measureRef{}, Removed: []measureRef{}, Changed: []measureRef{}}
	for _, measure := range after {
		ref := measureRef{measure.Number, measure.ShortName}
		previous, ok := old[measure.Number]
		switch {
		case !ok:
			changes.Added = append(changes.Added, ref)
		case !reflect.DeepEqual(previous, measure):
			changes.Changed = append(changes.Changed, ref)
		}
		delete(old, measure.Number)
	}
	for _, measure := range before {
		if _, gone := old[measure.Number]; gone {
			changes.Removed = append(changes.Removed, measureRef{measure.Number, measure.ShortName})
		}
	}
	return changes
}
