package handler

import "encoding/json"

// OptionalPatchInt kennzeichnet, ob "group_id" in JSON vorkommt: weggelassen = kein Update;
// null = Zuordnung entfernen; Zahl = setzen.
type OptionalPatchInt struct {
	Sent  bool
	Value *int
}

func (o *OptionalPatchInt) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		o.Sent = true
		o.Value = nil
		return nil
	}
	var v int
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Sent = true
	o.Value = &v
	return nil
}
