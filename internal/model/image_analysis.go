package model

// ImageEvidence is an explicitly labelled model transcription, never a text-body
// offset or a claim of independent visual verification.
type ImageEvidence struct {
	ImageURL    string `json:"image_url" bson:"image_url"`
	ImageIndex  int    `json:"image_index" bson:"image_index"`
	ImageSHA256 string `json:"image_sha256" bson:"image_sha256"`
	Text        string `json:"evidence_text" bson:"evidence_text"`
	Confidence  string `json:"confidence" bson:"confidence"`
	Validation  string `json:"validation" bson:"validation"`
}
