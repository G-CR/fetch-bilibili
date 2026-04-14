package repo

import "errors"

var ErrNotImplemented = errors.New("仓库功能未实现")
var ErrNotFound = errors.New("记录不存在")
var ErrConflict = errors.New("记录存在关联数据，无法删除")
