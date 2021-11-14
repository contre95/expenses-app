package importing

import (
	"errors"
	"expenses-app/pkg/app"
	"expenses-app/pkg/domain/expense"
	"fmt"
	"time"
)

type ImportExpensesResp struct {
	ID                string
	Msg               string
	SuccesfullImports int
	FailedImports     int
}

type ImportExpensesReq struct {
	BypassWrongExpenses bool
	ReImport            bool
	ImporterID          string
}

// IImportedExpense holds the values that should be imported by the Importer
type ImportedExpense struct {
	Amount   float64
	Currency string
	Product  string
	Shop     string
	Date     time.Time
	City     string
	Town     string
	People   string

	Category string
}

// Importer is the main dependency of the ImportExpenses and defines how an importer should behave
type Importer interface {
	//GetAllCategories() ([]string, error)
	GetImportedExpenses() ([]ImportedExpense, error)
}

// The ImportExpenses use case creates a category for a expense
type ImportExpenses struct {
	logger    app.Logger
	importers map[string]Importer
	expenses  expense.Expenses
}

// Contructor for Import
func NewExpenseImporter(l app.Logger, i map[string]Importer, e expense.Expenses) *ImportExpenses {
	return &ImportExpenses{l, i, e}
}

func parseExpense(e ImportedExpense) (*expense.Expense, error) {
	price := expense.Price{
		Currency: e.Currency,
		Amount:   e.Amount,
	}
	place := expense.Place{
		City: e.City,
		Town: e.Town,
		Shop: e.Shop,
	}
	return expense.NewExpense(price, e.Product, e.People, place, e.Date, e.Category)
}

// Import imports a all the categories provided by the importer
func (u *ImportExpenses) Import(req ImportExpensesReq) (*ImportExpensesResp, error) {
	importedExpenses, err := u.importers[req.ImporterID].GetImportedExpenses()
	if err != nil {
		u.logger.Err("Could not import expenses: %s", err)
		return nil, errors.New("Could not import expenses from importer" + req.ImporterID)
	}
	expensesToAdd := []expense.Expense{}
	failedExpenses := 0
	for _, e := range importedExpenses {
		newExp, err := parseExpense(e)
		if err != nil {
			failedExpenses++
			u.logger.Err("Could not import expense: %s of %f %s: %s", e.Product, e.Amount, e.Currency, err)
			if !req.BypassWrongExpenses {
				fmt.Println(req.BypassWrongExpenses)
				return nil, errors.New(fmt.Sprintf("Failed to import expense: %s of %f %s", e.Product, e.Amount, e.Currency))
			}
		} else {
			expensesToAdd = append(expensesToAdd, *newExp)
		}
	}
	for _, exp := range expensesToAdd {
		err := u.expenses.Add(exp)
		if err != nil {
			failedExpenses++
			u.logger.Err("Failed to save expense %s : %s", exp.ID, err)
			if !req.BypassWrongExpenses {
				fmt.Println(req.BypassWrongExpenses)
				return nil, errors.New(fmt.Sprintf("Failed to save expense %s : %s", exp.ID, err))
			}
		}
	}
	return &ImportExpensesResp{
		SuccesfullImports: len(importedExpenses) - failedExpenses,
		FailedImports:     failedExpenses,
	}, nil
}
