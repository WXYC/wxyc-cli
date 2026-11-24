package api

import (
	"context"
	"fmt"
)

// LabelReferences counts the rows pointing at a label, per source
// (Backend-Service LABEL_REFERENCES).
type LabelReferences struct {
	Library     int `json:"library"`
	Rotation    int `json:"rotation"`
	Flowsheet   int `json:"flowsheet"`
	IntakeItems int `json:"intake_items"`
}

// LabelWithReferences is GET /labels/{id}
// (labelsService.getLabelWithReferences): the label row plus how many rows
// reference it and how many labels name it as their parent.
type LabelWithReferences struct {
	Label
	References  LabelReferences `json:"references"`
	ChildLabels int             `json:"child_labels"`
}

// Label returns one label with its reference counts. A missing label is a 404.
func (c *Client) Label(ctx context.Context, id int) (LabelWithReferences, []byte, error) {
	var l LabelWithReferences
	raw, err := c.getInto(ctx, fmt.Sprintf("/labels/%d", id), nil, &l)
	return l, raw, err
}
