package sso_test

import (
	"encoding/xml"
	"testing"

	"github.com/crewjam/saml"
)

func TestSubjectRejectsForeignNamespaceNameID(t *testing.T) {
	for _, namespace := range []string{"urn:attacker", ""} {
		t.Run(namespace, func(t *testing.T) {
			var subject saml.Subject
			input := `<Subject xmlns="urn:oasis:names:tc:SAML:2.0:assertion"><NameID xmlns="` + namespace + `">admin@example.com</NameID></Subject>`
			if err := xml.Unmarshal([]byte(input), &subject); err != nil {
				t.Fatal(err)
			}
			if subject.NameID != nil {
				t.Fatalf("foreign NameID accepted as principal: %s", subject.NameID.Value)
			}
		})
	}
}

func TestSubjectAcceptsSAMLNamespaceNameID(t *testing.T) {
	var subject saml.Subject
	input := `<s:Subject xmlns:s="urn:oasis:names:tc:SAML:2.0:assertion"><s:NameID>user@example.com</s:NameID></s:Subject>`
	if err := xml.Unmarshal([]byte(input), &subject); err != nil {
		t.Fatal(err)
	}
	if subject.NameID == nil || subject.NameID.Value != "user@example.com" {
		t.Fatalf("SAML principal not decoded: %+v", subject.NameID)
	}
}
