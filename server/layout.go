package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bouk/httprouter"
	"github.com/influxdata/chronograf"
)

type link struct {
	Href string `json:"href"`
	Rel  string `json:"rel"`
}

type layoutResponse struct {
	chronograf.Layout
	Link link `json:"link"`
}

func newLayoutResponse(layout chronograf.Layout) layoutResponse {
	httpAPILayouts := "/chronograf/v1/layouts"
	href := fmt.Sprintf("%s/%s", httpAPILayouts, layout.ID)
	rel := "self"

	for idx, cell := range layout.Cells {
		axes := []string{"x", "y", "y2"}

		if cell.Axes == nil {
			layout.Cells[idx].Axes = make(map[string]chronograf.Axis, len(axes))
		}

		for _, axis := range axes {
			if _, found := cell.Axes[axis]; !found {
				layout.Cells[idx].Axes[axis] = chronograf.Axis{
					Bounds: []string{},
				}
			}
		}
	}

	return layoutResponse{
		Layout: layout,
		Link: link{
			Href: href,
			Rel:  rel,
		},
	}
}

// NewLayout adds a valid layout to store.
func (s *Service) NewLayout(w http.ResponseWriter, r *http.Request) {
	var layout chronograf.Layout
	if err := json.NewDecoder(r.Body).Decode(&layout); err != nil {
		invalidJSON(w, s.Logger)
		return
	}

	if err := ValidLayoutRequest(layout); err != nil {
		invalidData(w, err, s.Logger)
		return
	}

	var err error
	if layout, err = s.LayoutsStore.Add(r.Context(), layout); err != nil {
		msg := fmt.Errorf("Error storing layout %v: %v", layout, err)
		unknownErrorWithMessage(w, msg, s.Logger)
		return
	}

	res := newLayoutResponse(layout)
	location(w, res.Link.Href)
	encodeJSON(w, http.StatusCreated, res, s.Logger)
}

type getLayoutsResponse struct {
	Layouts []layoutResponse `json:"layouts"`
}

// Layouts retrieves all layouts from store
func (s *Service) Layouts(w http.ResponseWriter, r *http.Request) {
	// Construct a filter sieve for both applications and measurements
	filtered := map[string]bool{}
	for _, a := range r.URL.Query()["app"] {
		filtered[a] = true
	}

	for _, m := range r.URL.Query()["measurement"] {
		filtered[m] = true
	}

	ctx := r.Context()
	layouts, err := s.LayoutsStore.All(ctx)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Error loading layouts", s.Logger)
		return
	}

	filter := func(layout *chronograf.Layout) bool {
		// If the length of the filter is zero then all values are acceptable.
		if len(filtered) == 0 {
			return true
		}

		// If filter contains either measurement or application
		return filtered[layout.Measurement] || filtered[layout.Application]
	}

	res := getLayoutsResponse{
		Layouts: []layoutResponse{},
	}
	for _, layout := range layouts {
		if filter(&layout) {
			res.Layouts = append(res.Layouts, newLayoutResponse(layout))
		}
	}
	encodeJSON(w, http.StatusOK, res, s.Logger)
}

// LayoutsID retrieves layout with ID from store
func (s *Service) LayoutsID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httprouter.GetParamFromContext(ctx, "id")

	layout, err := s.LayoutsStore.Get(ctx, id)
	if err != nil {
		Error(w, http.StatusNotFound, fmt.Sprintf("ID %s not found", id), s.Logger)
		return
	}

	res := newLayoutResponse(layout)
	encodeJSON(w, http.StatusOK, res, s.Logger)
}

// RemoveLayout deletes layout from store.
func (s *Service) RemoveLayout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httprouter.GetParamFromContext(ctx, "id")

	layout := chronograf.Layout{
		ID: id,
	}

	if err := s.LayoutsStore.Delete(ctx, layout); err != nil {
		unknownErrorWithMessage(w, err, s.Logger)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateLayout replaces the layout of ID with new valid layout.
func (s *Service) UpdateLayout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := httprouter.GetParamFromContext(ctx, "id")

	_, err := s.LayoutsStore.Get(ctx, id)
	if err != nil {
		Error(w, http.StatusNotFound, fmt.Sprintf("ID %s not found", id), s.Logger)
		return
	}

	var req chronograf.Layout
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		invalidJSON(w, s.Logger)
		return
	}
	req.ID = id

	if err := ValidLayoutRequest(req); err != nil {
		invalidData(w, err, s.Logger)
		return
	}

	if err := s.LayoutsStore.Update(ctx, req); err != nil {
		msg := fmt.Sprintf("Error updating layout ID %s: %v", id, err)
		Error(w, http.StatusInternalServerError, msg, s.Logger)
		return
	}

	res := newLayoutResponse(req)
	encodeJSON(w, http.StatusOK, res, s.Logger)
}

// ValidLayoutRequest checks if the layout has valid application, measurement and cells.
func ValidLayoutRequest(l chronograf.Layout) error {
	if l.Application == "" || l.Measurement == "" || len(l.Cells) == 0 {
		return fmt.Errorf("app, measurement, and cells required")
	}

	for _, c := range l.Cells {
		if c.W == 0 || c.H == 0 {
			return fmt.Errorf("w, and h required")
		}
		for _, q := range c.Queries {
			if q.Command == "" {
				return fmt.Errorf("query required")
			}
		}
	}
	return nil
}
