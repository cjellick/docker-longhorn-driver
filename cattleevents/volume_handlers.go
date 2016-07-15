package cattleevents

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/mapstructure"

	"github.com/rancher/docker-longhorn-driver/driver"
	"github.com/rancher/docker-longhorn-driver/util"
	revents "github.com/rancher/event-subscriber/events"
	"github.com/rancher/go-rancher/client"
	"time"
)

type volumeHandlers struct {
	daemon *driver.StorageDaemon
}

func (h *volumeHandlers) RevertOrRestore(event *revents.Event, cli *client.RancherClient) error {
	logrus.Infof("Received event: Name: %s, Event Id: %s, Resource Id: %s", event.Name, event.ID, event.ResourceID)

	snapshot := &eventSnapshot{}
	err := decodeEvent(event, "snapshot", snapshot)
	if err != nil {
		return err
	}

	pd := &processData{}
	if err = decodeEvent(event, "processData", pd); err != nil {
		return err
	}

	if pd.Action == "revert" {
		logrus.Infof("Reverting to snapshot %v", snapshot.UUID)

		volClient := newVolumeClient(snapshot)

		_, err = volClient.revertToSnapshot(snapshot.UUID)
		if err != nil {
			return err
		}
		return reply("volume", event, cli)
	} else if pd.Action == "restore" {
		return h.restoreFromBackup(snapshot, pd, event, cli)
	} else {
		return fmt.Errorf("Unknown action: %v. Event: %#v", pd.Action, event)
	}
}

func (h *volumeHandlers) restoreFromBackup(snapshot *eventSnapshot, pd *processData, event *revents.Event, cli *client.RancherClient) error {
	volClient := newVolumeClientFromName(pd.VolumeName)

	logrus.Infof("Restoring from backup %v of snapshot %v.", snapshot.BackupURI, snapshot.UUID)

	target := newBackupTarget(snapshot)
	status, err := volClient.restoreFromBackup(pd.ProcessID, snapshot.BackupURI, target)
	if err != nil {
		return err
	}

	err = util.Backoff(time.Hour*12, fmt.Sprintf("Failed waiting for restore to backup: %v %v", snapshot.UUID, snapshot.BackupURI),
		func() (bool, error) {
			s, err := volClient.reloadStatus(status)
			if err != nil {
				return false, err
			}
			if s.State == "done" {
				return true, nil
			} else if s.State == "error" {
				return false, fmt.Errorf("Restore failed. Status: %v", s.Message)
			}
			return false, nil
		})

	if err != nil {
		return err
	}

	return reply("volume", event, cli)
}

func (h *volumeHandlers) VolumeRemove(event *revents.Event, cli *client.RancherClient) error {
	logrus.Infof("Received event: Name: %s, Event Id: %s, Resource Id: %s", event.Name, event.ID, event.ResourceID)

	vspm := &struct {
		VSPM struct {
			V struct {
				Name string `mapstructure:"name"`
			} `mapstructure:"volume"`
		} `mapstructure:"volumeStoragePoolMap"`
	}{}

	err := mapstructure.Decode(event.Data, &vspm)
	if err != nil {
		return fmt.Errorf("Cannot parse event %v. Error: %v", event, err)
	}

	name := vspm.VSPM.V.Name
	if name != "" {
		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf(deleteURL, name), nil)
		if err != nil {
			return fmt.Errorf("Error building delete request for %v: %v", name, err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("Error calling volume delete API for %v: %v", name, err)
		}

		if resp.StatusCode >= 300 {
			body, _ := ioutil.ReadAll(resp.Body)
			return fmt.Errorf("Unexpected repsonse code %v deleting %v. Body: %s", resp.StatusCode, name, body)
		}
	}

	return reply("volume", event, cli)
}
