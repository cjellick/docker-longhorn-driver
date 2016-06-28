package cattleevents

import (
	"encoding/json"
	"testing"

	revents "github.com/rancher/go-machine-service/events"
)

var snapshotEvent = `{"name":"asdf",
	"data":{"snapshot":{"name":"","id":3,"state":"creating","description":"","created":1464108273000,"data":{},"accountId":5,
	"volumeId":126,"kind":"snapshot","removed":"","uuid":"snap-uuid", "type":"snapshot",
	"volume":{"name":"expected-vol-name","id":126,"state":"active","created":1464107700000,
	"data":{"fields":{"driver":"longhorn","capabilities":["snapshot"]}},"accountId":5,"kind":"volume","zoneId":1,"hostId":1,
	"uuid":"expected-vol-uuid","externalId":"vd1","accessMode":"singleHostRW",
	"allocationState":"active","type":"volume"}}
	}}`

var backupEvent = `
{
  "id": "83255ac2-abab-4098-a62a-8e87bc2bb3d7",
  "name": "storage.snapshot.backup",
  "replyTo": "reply.416791149553575714",
  "resourceId": "1s2",
  "resourceType": "snapshot",
  "publisher": null,
  "transitioning": null,
  "transitioningMessage": null,
  "transitioningInternalMessage": null,
  "previousIds": null,
  "previousNames": null,
  "data": {
    "snapshot": {
      "name": "aaaa",
      "id": 2,
      "state": "backingup",
      "kind": "snapshot",
      "accountId": 5,
      "data": {},
      "uuid": "4dceeb1f-9623-4be0-b1ff-6422de7ffc82",
      "volumeId": 34,
      "backupTargetId": 1,
      "type": "snapshot",
      "volume": {
        "name": "foo1",
        "id": 34
      },
      "backupTarget": {
        "name": "name",
        "id": 1,
        "state": "created",
        "created": 1465276622000,
        "uuid": "auuid",
        "accountId": 5,
        "kind": "backupTarget",
        "data": {
          "fields": {
            "nfsConfig": {
              "server": "1.2.3.5",
              "share": "/var/nfs"
            }
          }
        },
        "type": "backupTarget"
      }
    }
  }
}
`

func TestSnapshotEvent(t *testing.T) {
	event := createEvent(snapshotEvent, t)

	snapshotData := &eventSnapshot{}
	err := decodeEvent(event, "snapshot", snapshotData)
	if err != nil {
		t.Fatal(err)
	}

	if snapshotData.Volume.Name != "expected-vol-name" || snapshotData.Volume.UUID != "expected-vol-uuid" {
		t.Fatalf("Unexpected: %v %v", snapshotData.Volume.Name, snapshotData.Volume.UUID)
	}
}

func TestBackupEvent(t *testing.T) {
	event := createEvent(backupEvent, t)

	backupData := &eventSnapshot{}
	err := decodeEvent(event, "snapshot", backupData)
	if err != nil {
		t.Fatal(err)
	}

	conf := backupData.BackupTarget.Data.Fields.NFSConfig
	if backupData.BackupTarget.Name != "name" || backupData.BackupTarget.UUID != "auuid" ||
		conf.Server != "1.2.3.5" || conf.Share != "/var/nfs" || conf.MountOptions != "" {
		t.Fatalf("Unexpected: %v", backupData.BackupTarget)
	}
}

func createEvent(eventData string, t *testing.T) *revents.Event {
	eventJSON := []byte(eventData)
	event := &revents.Event{}
	err := json.Unmarshal(eventJSON, event)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return event
}
