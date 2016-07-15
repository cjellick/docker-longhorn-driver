package cattleevents

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/rancher/convoy/objectstore"
	"github.com/rancher/docker-longhorn-driver/util"
	revents "github.com/rancher/event-subscriber/events"
	"github.com/rancher/go-rancher/client"
)

type notExistError error

var backupDoesntExist notExistError

var doesntExistRegex = regexp.MustCompile("cannot find.*in objectstore")

type backupHandlers struct {
}

func (h *backupHandlers) Create(event *revents.Event, cli *client.RancherClient) error {
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

	volClient := newVolumeClient(snapshot)

	logrus.Infof("Creating backup of snapshot %v", snapshot.UUID)

	target := newBackupTarget(snapshot)
	status, err := volClient.createBackup(pd.ProcessID, snapshot.UUID, target)
	if err != nil {
		return err
	}

	err = util.Backoff(time.Hour*12, fmt.Sprintf("Failed waiting for restore to backup :%s", "somedir"), func() (bool, error) {
		s, err := volClient.reloadStatus(status)
		if err != nil {
			return false, err
		}
		if s.State == "done" {
			return true, nil
		} else if s.State == "error" {
			return false, fmt.Errorf("Backup create failed. Status: %v", s.Message)
		}
		return false, nil
	})

	if err != nil {
		return err
	}

	status, err = volClient.reloadStatus(status)
	if err != nil {
		return err
	}

	uri := strings.TrimSpace(status.Message)
	backupUpdates := map[string]interface{}{"backupUri": uri}
	eventDataWrapper := map[string]interface{}{"snapshot": backupUpdates}

	reply := newReply(event)
	reply.ResourceType = "snapshot"
	reply.ResourceId = event.ResourceID
	reply.Data = eventDataWrapper

	logrus.Infof("Reply: %+v", reply)
	return publishReply(reply, cli)

}

func (h *backupHandlers) Delete(event *revents.Event, cli *client.RancherClient) error {
	logrus.Infof("Received event: Name: %s, Event Id: %s, Resource Id: %s", event.Name, event.ID, event.ResourceID)

	snapshot := &eventSnapshot{}
	if err := decodeEvent(event, "snapshot", snapshot); err != nil {
		return err
	}

	if err := removeBackup(snapshot); err != nil {
		if err == backupDoesntExist {
			logrus.Infof("Backup %v already removed.", snapshot.BackupURI)
			return publishRemoveBackupReply(event, cli)
		}
		return errors.Wrapf(err, "Error removing backup %v for snapshot %v.", snapshot.BackupURI, event.ResourceID)
	}

	return publishRemoveBackupReply(event, cli)
}

func removeBackup(snapshot *eventSnapshot) error {
	target := newBackupTarget(snapshot)
	if err := createNFSMount(target); err != nil {
		return errors.Wrapf(err, "Unable to create nfs mount for %v.", target)
	}

	logrus.Infof("Checking if backup %v exists.", snapshot.BackupURI)
	if _, err := objectstore.GetBackupInfo(snapshot.BackupURI); err != nil {
		if doesntExistRegex.MatchString(err.Error()) {
			return backupDoesntExist
		}
		return errors.Wrapf(err, "Couldn't determine if error exists.")
	}

	logrus.Infof("Removing backup %v for snapshot %v", snapshot.BackupURI, snapshot.UUID)
	if err := objectstore.DeleteDeltaBlockBackup(snapshot.BackupURI); err != nil {
		return errors.Wrapf(err, "Error deleting backup.")
	}

	return nil
}

func publishRemoveBackupReply(event *revents.Event, cli *client.RancherClient) error {
	reply := newReply(event)
	reply.ResourceType = "snapshot"
	reply.ResourceId = event.ResourceID
	backupUpdates := map[string]interface{}{"backupUri": nil, "backupTargetId": nil}
	eventDataWrapper := map[string]interface{}{"snapshot": backupUpdates}
	reply.Data = eventDataWrapper

	logrus.Infof("Reply: %+v", reply)
	return publishReply(reply, cli)
}

func newBackupTarget(snapshot *eventSnapshot) backupTarget {
	return backupTarget{
		Name:      snapshot.BackupTarget.Name,
		UUID:      snapshot.BackupTarget.UUID,
		NFSConfig: snapshot.BackupTarget.Data.Fields.NFSConfig,
	}
}

func createNFSMount(target backupTarget) error {
	mountDir := constructMountDir(target)

	grep := exec.Command("grep", mountDir, "/proc/mounts")
	if err := grep.Run(); err == nil {
		logrus.Infof("Found mount %v.", mountDir)
		return nil
	}

	if err := os.MkdirAll(mountDir, 0770); err != nil {
		return err
	}

	remoteTarget := fmt.Sprintf("%v:%v", target.NFSConfig.Server, target.NFSConfig.Share)
	parentPid := strconv.Itoa(os.Getppid())

	var mount *exec.Cmd
	if target.NFSConfig.MountOptions == "" {
		mount = exec.Command("nsenter", "-t", parentPid, "-n", "mount", "-t", "nfs", remoteTarget, mountDir)
	} else {
		mount = exec.Command("nsenter", "-t", parentPid, "-n", "mount", "-t", "nfs", "-o", target.NFSConfig.MountOptions, remoteTarget, mountDir)
	}

	mount.Stdout = os.Stdout
	mount.Stderr = os.Stderr

	logrus.Infof("Running %v", mount.Args)
	return mount.Run()
}

func constructMountDir(target backupTarget) string {
	return fmt.Sprintf("/var/lib/rancher/longhorn/backups/%s/%s", target.Name, target.UUID)
}
