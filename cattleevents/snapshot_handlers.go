package cattleevents

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	revents "github.com/rancher/event-subscriber/events"
	"github.com/rancher/go-rancher/client"
)

type snapshotHandlers struct {
}

func (h *snapshotHandlers) Create(event *revents.Event, cli *client.RancherClient) error {
	logrus.Infof("Received event: %s, Event Id: %s, Resource Id: %s", event.Name, event.ID, event.ResourceID)

	snapshot := &eventSnapshot{}
	err := decodeEvent(event, "snapshot", snapshot)
	if err != nil {
		return err
	}

	volClient := newVolumeClient(snapshot)

	found, _ := volClient.getSnapshot(snapshot.UUID)
	if found != nil {
		return reply("snapshot", event, cli)
	}

	logrus.Infof("Creating snapshot %v", snapshot.UUID)

	if _, err := volClient.createSnapshot(snapshot.UUID); err != nil {
		return err
	}

	return reply("snapshot", event, cli)
}

func (h *snapshotHandlers) DeleteLocalAndBackup(event *revents.Event, cli *client.RancherClient) error {
	logrus.Infof("Received event: %s, Event Id: %s, Resource Id: %s", event.Name, event.ID, event.ResourceID)

	snapshot := &eventSnapshot{}
	err := decodeEvent(event, "snapshot", snapshot)
	if err != nil {
		return errors.Wrapf(err, "Error decoding snapshot event %v.", event)
	}

	if snapshot.BackupURI != "" {
		if err := removeBackup(snapshot); err != nil {
			if err == backupDoesntExist {
				logrus.Infof("Backup %v already removed.", snapshot.BackupURI)
			} else {
				return errors.Wrapf(err, "Error removing backup %v for snapshot %v.", snapshot.BackupURI, event.ResourceID)
			}
		}
	}

	if err := deleteSnapshot(snapshot); err != nil {
		if err == errInvalidVolumeState {
			logrus.Infof("Can't delete snapshot %v because volume is in state %v.", event.ResourceID, snapshot.Volume.State)
			return nil
		}
		return errors.Wrapf(err, "Error deleting snapshot %v, event %v", event.ResourceID, event.ID)
	}

	return publishRemoveBackupReply(event, cli)
}

func (h *snapshotHandlers) Delete(event *revents.Event, cli *client.RancherClient) error {
	logrus.Infof("Received event: %s, Event Id: %s, Resource Id: %s", event.Name, event.ID, event.ResourceID)

	snapshot := &eventSnapshot{}
	err := decodeEvent(event, "snapshot", snapshot)
	if err != nil {
		return errors.Wrapf(err, "Error decoding snapshot event %v.", event)
	}

	if err := deleteSnapshot(snapshot); err != nil {
		if err == errInvalidVolumeState {
			logrus.Infof("Can't delete snapshot %v because volume is in state %v.", event.ResourceID, snapshot.Volume.State)
			return nil
		}
		return errors.Wrapf(err, "Error deleting snapshot %v, event %v", event.ResourceID, event.ID)
	}

	return reply("snapshot", event, cli)
}

func deleteSnapshot(snapshot *eventSnapshot) error {
	if snapshot.Volume.Removed != 0 {
		return nil
	}

	if snapshot.Volume.State != "active" && snapshot.Volume.State != "inactive" {
		return errInvalidVolumeState
	}

	volClient := newVolumeClient(snapshot)
	if err := volClient.deleteSnapshot(snapshot.UUID); err != nil {
		return errors.Wrapf(err, "Error deleting snapshot %v", snapshot.UUID)
	}

	return nil
}

var errInvalidVolumeState = fmt.Errorf("Volume is in invalid state for deleting snapshot.")
