package util

import (
	"testing"
)

func TestConvertSize(t *testing.T) {

	size, sizeGB, err := ConvertSize("0b")
	if err != nil {
		t.Fatalf("Couldn't parse zero. Error: %v", err)
	}

	if size != "0" {
		t.Fatalf("Size is: %v. Expected 0", size)
	}

	if sizeGB != "1" {
		t.Fatalf("SizeGB is: %v. Expected 1", size)
	}

	size, sizeGB, err = ConvertSize("1024b")
	if err != nil {
		t.Fatalf("Couldn't parse zero. Error: %v", err)
	}

	if size != "1024" {
		t.Fatalf("Size is: %v. Expected 0", size)
	}

	if sizeGB != "1" {
		t.Fatalf("SizeGB is: %v. Expected 1", size)
	}

	size, sizeGB, err = ConvertSize("1024")
	if err == nil {
		t.Fatalf("Expected error parsing size because no unit was specified.")
	}
}
