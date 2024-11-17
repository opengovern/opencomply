package db

import (
	"errors"
	"time"

	runner "github.com/opengovern/opengovernance/jobs/compliance-runner"
	"github.com/opengovern/opengovernance/services/describe/db/model"
	"gorm.io/gorm"
)

func (db Database) CreateRunnerJobs(tx *gorm.DB, runners []*model.ComplianceRunner) error {
	if tx == nil {
		tx = db.ORM
	}
	tx = tx.
		Model(&model.ComplianceRunner{}).
		CreateInBatches(runners, 500)
	if tx.Error != nil {
		return tx.Error
	}

	return nil
}

func (db Database) DeleteOldRunnerJob(parentJobId *uint) error {
	tx := db.ORM.Model(&model.ComplianceRunner{})
	if parentJobId != nil {
		tx = tx.Where("parent_job_id = ?", *parentJobId)
	} else {
		tx = tx.Where("created_at < ?", time.Now().Add(-time.Hour*24*2))
	}
	tx = tx.Unscoped().Delete(&model.ComplianceRunner{})
	if tx.Error != nil {
		return tx.Error
	}

	tx = db.ORM.Model(&model.ComplianceRunner{}).
		Where("created_at < ?", time.Now().Add(-time.Hour*24*7)).
		Unscoped().Delete(&model.ComplianceRunner{})
	if tx.Error != nil {
		return tx.Error
	}

	return nil
}

func (db Database) FetchCreatedRunners(manual bool) ([]model.ComplianceRunner, error) {
	var jobs []model.ComplianceRunner
	tx := db.ORM.Model(&model.ComplianceRunner{}).
		Where("status = ?", runner.ComplianceRunnerCreated)

	if manual {
		tx = tx.Where("trigger_type = ?", model.ComplianceTriggerTypeManual)
	} else {
		tx = tx.Where("trigger_type <> ?", model.ComplianceTriggerTypeManual)
	}

	tx = tx.Order("created_at ASC").Limit(1000).Find(&jobs)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, tx.Error
	}
	return jobs, nil
}

func (db Database) UpdateTimedOutInProgressRunners() error {
	tx := db.ORM.
		Model(&model.ComplianceRunner{}).
		Where("status = ?", runner.ComplianceRunnerInProgress).
		Where("updated_at < NOW() - INTERVAL '1 HOURS'").
		Updates(model.ComplianceRunner{Status: runner.ComplianceRunnerTimeOut, FailureMessage: "Job timed out"})
	if tx.Error != nil {
		return tx.Error
	}

	return nil
}

func (db Database) RetryFailedRunners() error {
	tx := db.ORM.Exec("UPDATE compliance_runners SET retry_count = retry_count + 1, status = 'CREATED', updated_at = NOW() WHERE status = 'FAILED' AND retry_count < 1 AND updated_at < NOW() - interval '5 minutes'")
	if tx.Error != nil {
		return tx.Error
	}

	return nil
}

func (db Database) UpdateRunnerJob(
	id uint, status runner.ComplianceRunnerStatus, startedAt time.Time, totalFindingCount *int, failureMsg string) error {
	tx := db.ORM.
		Model(&model.ComplianceRunner{}).
		Where("id = ?", id).
		Updates(model.ComplianceRunner{
			Status:            status,
			StartedAt:         startedAt,
			FailureMessage:    failureMsg,
			TotalFindingCount: totalFindingCount,
		})
	if tx.Error != nil {
		return tx.Error
	}

	return nil
}

func (db Database) UpdateRunnerJobNatsSeqNum(
	id uint, seqNum uint64) error {
	tx := db.ORM.
		Model(&model.ComplianceRunner{}).
		Where("id = ?", id).
		Updates(model.ComplianceRunner{
			NatsSequenceNumber: seqNum,
		})
	if tx.Error != nil {
		return tx.Error
	}

	return nil
}

func (db Database) UpdateTimeoutQueuedRunnerJobs() error {
	tx := db.ORM.
		Model(&model.ComplianceRunner{}).
		Where("created_at < NOW() - INTERVAL '12 HOURS'").
		Where("status IN ?", []string{string(runner.ComplianceRunnerCreated), string(runner.ComplianceRunnerQueued)}).
		Updates(model.ComplianceRunner{Status: runner.ComplianceRunnerTimeOut, FailureMessage: "Job timed out"})
	if tx.Error != nil {
		return tx.Error
	}

	return nil
}

func (db Database) ListRunnersWithID(ids []int64) ([]model.ComplianceRunner, error) {
	var jobs []model.ComplianceRunner
	tx := db.ORM.Where("id IN ?", ids).Find(&jobs)
	if tx.Error != nil {
		return nil, tx.Error
	}

	return jobs, nil
}

func (db Database) ListFailedRunnersWithParentID(id uint) ([]model.ComplianceRunner, error) {
	var jobs []model.ComplianceRunner
	tx := db.ORM.Model(&model.ComplianceRunner{}).
		Where("status = ?", runner.ComplianceRunnerFailed).
		Where("parent_job_id = ?", id).
		Find(&jobs)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, tx.Error
	}
	return jobs, nil
}

func (db Database) ListComplianceJobRunnersWithID(id uint) ([]model.ComplianceRunner, error) {
	var jobs []model.ComplianceRunner
	tx := db.ORM.Where("parent_job_id = ?", id).Find(&jobs)
	if tx.Error != nil {
		return nil, tx.Error
	}

	return jobs, nil
}

func (db Database) CleanupAllComplianceRunners() error {
	tx := db.ORM.Where("1 = 1").Unscoped().Delete(&model.ComplianceRunner{})
	if tx.Error != nil {
		return tx.Error
	}
	return nil
}
