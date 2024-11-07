// Copyright (c) 2020 Mercari, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package internal

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kenshaw/snaker"
	"go.mercari.io/yo/models"
	"gopkg.in/yaml.v2"
)

type loaderImpl interface {
	ParamN(int) string
	MaskFunc() string
	ParseType(string, bool) (int, string, string)
	ValidCustomType(string, string) bool
	TableList() ([]*models.Table, error)
	ColumnList(string) ([]*models.Column, error)
	IndexList(string) ([]*models.Index, error)
	IndexColumnList(string, string) ([]*models.IndexColumn, error)
}

func NewTypeLoader(l loaderImpl, i Inflector, s *snaker.Initialisms) *TypeLoader {
	return &TypeLoader{loader: l, inflector: i, snaker: s}
}

// TypeLoader provides a common Loader implementation used by the built in
// schema/query loaders.
type TypeLoader struct {
	CustomTypes *models.CustomTypes
	loader      loaderImpl
	inflector   Inflector
	snaker      *snaker.Initialisms
}

// NthParam satisifies Loader's NthParam.
func (tl *TypeLoader) NthParam(i int) string {
	return tl.loader.ParamN(i)
}

// Mask returns the parameter mask.
func (tl *TypeLoader) Mask() string {

	return tl.loader.MaskFunc()
}

// LoadSchema loads schema definitions.
func (tl *TypeLoader) LoadSchema(args *ArgType) (map[string]*Type, map[string]*Index, error) {
	var err error

	// load tables
	tableMap, err := tl.LoadTable(args)
	if err != nil {
		return nil, nil, err
	}

	// load indexes
	ixMap, err := tl.LoadIndexes(args, tableMap)
	if err != nil {
		return nil, nil, err
	}

	setIndexesToTables(tableMap, ixMap)

	return tableMap, ixMap, nil
}

// LoadTable loads a schema table/view definition.
func (tl *TypeLoader) LoadTable(args *ArgType) (map[string]*Type, error) {
	var err error

	// load tables
	tableList, err := tl.loader.TableList()
	if err != nil {
		return nil, err
	}

	// tables
	tableMap := make(map[string]*Type)
	for _, ti := range tableList {
		ignore := false

		// Ignore tables specified by IgnoreTables argument.
		for _, ignoreTable := range args.IgnoreTables {
			if ignoreTable == ti.TableName {
				// Skip adding this table if user has specified they are not
				// interested.
				//
				// This could be useful for tables which are managed by the
				// database (e.g. SchemaMigrations) instead of
				// via Go code.
				ignore = true
			}
		}

		// If the 'TargetTables' argument is passed, ignore any tables that are not specified in the array.
		if len(args.TargetTables) != 0 {
			ignore = true
			for _, t := range args.TargetTables {
				if t == ti.TableName {
					ignore = false
				}
			}
		}

		if ignore {
			continue
		}

		// create template
		typeTpl := &Type{
			Name:   SingularizeIdentifier(tl.inflector, ti.TableName),
			Schema: "",
			Fields: []*Field{},
			Table:  ti,
		}

		// process columns
		err = tl.LoadColumns(args, typeTpl)
		if err != nil {
			return nil, err
		}

		if err := tl.loadPrimaryKeys(typeTpl); err != nil {
			return nil, err
		}

		tableMap[ti.TableName] = typeTpl
	}

	// validate custom type tables
	if tl.CustomTypes != nil {
		for _, customTable := range tl.CustomTypes.Tables {
			_, ok := tableMap[customTable.Name]
			if !ok {
				return nil, fmt.Errorf("unknown custom type table: %s", customTable.Name)
			}
		}
	}

	return tableMap, nil
}

// loadPrimaryKeys loads primary key fields
func (tl *TypeLoader) loadPrimaryKeys(typeTpl *Type) error {
	// reorder primary keys
	indexCols, err := tl.loader.IndexColumnList(typeTpl.Table.TableName, "PRIMARY_KEY")
	if err != nil {
		panic(err)
	}

	var fields []*Field
	for _, idx := range indexCols {
		var field *Field
		for _, f := range typeTpl.Fields {
			if f.Col.ColumnName == idx.ColumnName {
				field = f
				break
			}
		}

		if field == nil {
			return fmt.Errorf("primary key column is not found in column list: table=%v column=%v",
				typeTpl.Name, idx.ColumnName,
			)
		}
		fields = append(fields, field)
	}

	typeTpl.PrimaryKey = fields[0] // backward compatibility
	typeTpl.PrimaryKeyFields = fields
	return nil
}

// tableCustomTypes find custom type definitions of the table
func (tl *TypeLoader) tableCustomTypes(table string) map[string]string {
	var columnTypes map[string]string
	if tl.CustomTypes != nil {
		for _, v := range tl.CustomTypes.Tables {
			if v.Name == table {
				columnTypes = v.Columns
				break
			}
		}
	}

	return columnTypes
}

// LoadColumns loads schema table/view columns.
func (tl *TypeLoader) LoadColumns(args *ArgType, typeTpl *Type) error {
	var err error

	// load columns
	columnList, err := tl.loader.ColumnList(typeTpl.Table.TableName)
	if err != nil {
		return err
	}

	columnTypes := tl.tableCustomTypes(typeTpl.Table.TableName)

	// validate custom type columns
	if columnTypes != nil {
		columnSet := map[string]struct{}{}
		for _, column := range columnList {
			columnSet[column.ColumnName] = struct{}{}
		}

		for k, _ := range columnTypes {
			if _, ok := columnSet[k]; !ok {
				return fmt.Errorf("unknown custom type column %s in the table %s", k, typeTpl.Table.TableName)
			}
		}
	}

	// process columns
	for _, c := range columnList {
		ignore := false

		for _, ignoreField := range args.IgnoreFields {
			if strings.Contains(ignoreField, ".") {
				s := strings.Split(ignoreField, ".")
				if s[0] == typeTpl.Table.TableName && s[1] == c.ColumnName {
					ignore = true
				}
			} else if ignoreField == c.ColumnName {
				// Skip adding this field if user has specified they are not
				// interested.
				//
				// This could be useful for fields which are managed by the
				// database (e.g. automatically updated timestamps) instead of
				// via Go code.
				ignore = true
			}
		}

		if ignore {
			continue
		}

		// set col info
		f := &Field{
			Name: tl.snaker.ForceCamelIdentifier(c.ColumnName),
			// Name: c.ColumnName,
			Col: c,
		}

		f.Len, f.NilType, f.Type = tl.loader.ParseType(c.DataType, !c.NotNull)

		// set custom type
		if columnTypes != nil {
			if t, ok := columnTypes[c.ColumnName]; ok && tl.loader.ValidCustomType(c.DataType, t) {
				f.CustomType = t
			}
		}

		// append col to template fields
		typeTpl.Fields = append(typeTpl.Fields, f)
	}

	return nil
}

// LoadIndexes loads schema index definitions.
func (tl *TypeLoader) LoadIndexes(args *ArgType, tableMap map[string]*Type) (map[string]*Index, error) {
	var err error

	ixMap := map[string]*Index{}
	for _, t := range tableMap {
		// load table indexes
		err = tl.LoadTableIndexes(args, t, ixMap)
		if err != nil {
			return nil, err
		}
	}

	return ixMap, nil
}

// LoadTableIndexes loads schema index definitions per table.
func (tl *TypeLoader) LoadTableIndexes(args *ArgType, typeTpl *Type, ixMap map[string]*Index) error {
	var err error
	var priIxLoaded bool

	// load indexes
	indexList, err := tl.loader.IndexList(typeTpl.Table.TableName)
	if err != nil {
		return err
	}

	// process indexes
	for _, ix := range indexList {
		// save whether or not the primary key index was processed
		priIxLoaded = priIxLoaded || ix.IsPrimary || (ix.Origin == "pk")

		// create index template
		ixTpl := &Index{
			Schema: "",
			Type:   typeTpl,
			Fields: []*Field{},
			Index:  ix,
		}

		// load index columns
		err = tl.LoadIndexColumns(args, ixTpl)
		if err != nil {
			return err
		}

		// build func name
		ixTpl.FuncName = tl.buildIndexFuncName(ixTpl)

		ixMap[typeTpl.Table.TableName+"_"+ix.IndexName] = ixTpl
	}

	return nil
}

func (tl *TypeLoader) buildIndexFuncName(ixTpl *Index) string {
	// build func name
	funcName := ixTpl.Type.Name
	if !ixTpl.Index.IsUnique {
		funcName = tl.inflector.Pluralize(ixTpl.Type.Name)
	}
	funcName = funcName + "By"

	// add param names
	paramNames := make([]string, 0, len(ixTpl.Fields))
	for _, f := range ixTpl.StoringFields {
		paramNames = append(paramNames, f.Name)
	}
	for _, f := range ixTpl.Fields {
		paramNames = append(paramNames, f.Name)
	}

	return funcName + strings.Join(paramNames, "")
}

// LoadIndexColumns loads the index column information.
func (tl *TypeLoader) LoadIndexColumns(args *ArgType, ixTpl *Index) error {
	var err error

	// load index columns
	indexCols, err := tl.loader.IndexColumnList(ixTpl.Type.Table.TableName, ixTpl.Index.IndexName)
	if err != nil {
		return err
	}

	// process index columns
	for _, ic := range indexCols {
		var field *Field

	fieldLoop:
		// find field
		for _, f := range ixTpl.Type.Fields {
			if f.Col.ColumnName == ic.ColumnName {
				field = f
				break fieldLoop
			}
		}

		if field == nil {
			continue
		}

		if ic.Storing {
			// Storing column is added to StoringFields
			ixTpl.StoringFields = append(ixTpl.StoringFields, field)
		} else {
			ixTpl.Fields = append(ixTpl.Fields, field)
		}
		if !field.Col.NotNull {
			ixTpl.NullableFields = append(ixTpl.NullableFields, field)
		}
	}

	return nil
}

// LoadCustomTypes loads custom type definition from yml file
func (tl *TypeLoader) LoadCustomTypes(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	var ctypes models.CustomTypes
	if err := yaml.NewDecoder(file).Decode(&ctypes); err != nil {
		return err
	}

	tl.CustomTypes = &ctypes

	return nil
}

func setIndexesToTables(tableMap map[string]*Type, ixMap map[string]*Index) {
	indexes := make([]*Index, 0, len(ixMap))
	for _, ix := range ixMap {
		indexes = append(indexes, ix)
	}
	sort.Slice(indexes, func(i, j int) bool {
		return indexes[i].FuncName < indexes[j].FuncName
	})
	for tbl, t := range tableMap {
		for _, ix := range indexes {
			if ix.Type.Table.TableName == tbl {
				t.Indexes = append(t.Indexes, ix)
			}
		}
	}
}
