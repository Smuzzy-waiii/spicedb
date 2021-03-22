package postgres

import (
	dbsql "database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/authzed/spicedb/internal/datastore"
	pb "github.com/authzed/spicedb/pkg/REDACTEDapi/api"
)

const errUnableToWriteTuples = "unable to write tuples: %w"

var (
	writeTuple = psql.Insert(tableTuple).Columns(
		colNamespace,
		colObjectID,
		colRelation,
		colUsersetNamespace,
		colUsersetObjectID,
		colUsersetRelation,
		colCreatedTxn,
	)

	deleteTuple = psql.Update(tableTuple).Where(sq.Eq{colDeletedTxn: liveDeletedTxnID})

	queryTupleExists = psql.Select(colID).From(tableTuple)
)

func (pgd *pgDatastore) WriteTuples(preconditions []*pb.RelationTuple, mutations []*pb.RelationTupleUpdate) (uint64, error) {
	tx, err := pgd.db.Beginx()
	if err != nil {
		return 0, fmt.Errorf(errUnableToWriteTuples, err)
	}
	defer tx.Rollback()

	// Check the preconditions
	for _, tpl := range preconditions {
		sql, args, err := queryTupleExists.Where(exactTupleClause(tpl)).Limit(1).ToSql()
		if err != nil {
			return 0, fmt.Errorf(errUnableToWriteTuples, err)
		}

		foundID := -1
		if err := tx.QueryRowx(sql, args...).Scan(&foundID); err != nil {
			if err == dbsql.ErrNoRows {
				return 0, datastore.ErrPreconditionFailed
			}
			return 0, fmt.Errorf(errUnableToWriteTuples, err)
		}
	}

	newTxnID, err := createNewTransaction(tx)
	if err != nil {
		return 0, fmt.Errorf(errUnableToWriteTuples, err)
	}

	bulkWrite := writeTuple
	bulkWriteHasValues := false

	// Process the actual updates
	for _, mutation := range mutations {
		tpl := mutation.Tuple
		if mutation.Operation == pb.RelationTupleUpdate_TOUCH || mutation.Operation == pb.RelationTupleUpdate_DELETE {
			sql, args, err := deleteTuple.Where(exactTupleClause(tpl)).Set(colDeletedTxn, newTxnID).ToSql()
			if err != nil {
				return 0, fmt.Errorf(errUnableToWriteTuples, err)
			}

			result, err := tx.Exec(sql, args...)
			if err != nil {
				return 0, fmt.Errorf(errUnableToWriteTuples, err)
			}

			affected, err := result.RowsAffected()
			if err != nil {
				return 0, fmt.Errorf(errUnableToWriteTuples, err)
			}

			if affected != 1 && mutation.Operation == pb.RelationTupleUpdate_DELETE {
				return 0, datastore.ErrPreconditionFailed
			}
		}

		if mutation.Operation == pb.RelationTupleUpdate_TOUCH || mutation.Operation == pb.RelationTupleUpdate_CREATE {
			bulkWrite = bulkWrite.Values(
				tpl.ObjectAndRelation.Namespace,
				tpl.ObjectAndRelation.ObjectId,
				tpl.ObjectAndRelation.Relation,
				tpl.User.GetUserset().Namespace,
				tpl.User.GetUserset().ObjectId,
				tpl.User.GetUserset().Relation,
				newTxnID,
			)
			bulkWriteHasValues = true
		}
	}

	if bulkWriteHasValues {
		sql, args, err := bulkWrite.ToSql()
		if err != nil {
			return 0, fmt.Errorf(errUnableToWriteTuples, err)
		}

		_, err = tx.Exec(sql, args...)
		if err != nil {
			return 0, fmt.Errorf(errUnableToWriteTuples, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return 0, fmt.Errorf(errUnableToWriteTuples, err)
	}

	return newTxnID, nil
}

func exactTupleClause(tpl *pb.RelationTuple) sq.Eq {
	return sq.Eq{
		colNamespace:        tpl.ObjectAndRelation.Namespace,
		colObjectID:         tpl.ObjectAndRelation.ObjectId,
		colRelation:         tpl.ObjectAndRelation.Relation,
		colUsersetNamespace: tpl.User.GetUserset().Namespace,
		colUsersetObjectID:  tpl.User.GetUserset().ObjectId,
		colUsersetRelation:  tpl.User.GetUserset().Relation,
	}
}