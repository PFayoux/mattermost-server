// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"bytes"
	"io"
	"net/http"

	"github.com/mattermost/mattermost-server/v5/audit"
	"github.com/mattermost/mattermost-server/v5/model"
)

func (api *API) InitLicense() {
	api.BaseRoutes.ApiRoot.HandleWithMiddleware("/license", addLicense,
		requireSession(),
		requireSystemPermissions(model.PERMISSION_MANAGE_SYSTEM),
		restrictSystemAdmin(),
	).Methods("POST")

	api.BaseRoutes.ApiRoot.HandleWithMiddleware("/license", removeLicense,
		requireSession(),
		requireSystemPermissions(model.PERMISSION_MANAGE_SYSTEM),
		restrictSystemAdmin(),
	).Methods("DELETE")

	api.BaseRoutes.ApiRoot.HandleWithMiddleware("/license/client", getClientLicense,
		requireQueryParam("format"),
		requireQueryInSet("format", []string{"old"}),
	).Methods("GET")
}

func getClientLicense(c *Context, w http.ResponseWriter, r *http.Request) {
	var clientLicense map[string]string

	if c.App.SessionHasPermissionTo(*c.App.Session(), model.PERMISSION_MANAGE_SYSTEM) {
		clientLicense = c.App.ClientLicense()
	} else {
		clientLicense = c.App.GetSanitizedClientLicense()
	}

	w.Write([]byte(model.MapToJson(clientLicense)))
}

func addLicense(c *Context, w http.ResponseWriter, r *http.Request) {
	auditRec := c.MakeAuditRecord("addLicense", audit.Fail)
	defer c.LogAuditRec(auditRec)
	c.LogAudit("attempt")

	err := r.ParseMultipartForm(*c.App.Config().FileSettings.MaxFileSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m := r.MultipartForm

	fileArray, ok := m.File["license"]
	if !ok {
		c.Err = model.NewAppError("addLicense", "api.license.add_license.no_file.app_error", nil, "", http.StatusBadRequest)
		return
	}

	if len(fileArray) <= 0 {
		c.Err = model.NewAppError("addLicense", "api.license.add_license.array.app_error", nil, "", http.StatusBadRequest)
		return
	}

	fileData := fileArray[0]
	auditRec.AddMeta("filename", fileData.Filename)

	file, err := fileData.Open()
	if err != nil {
		c.Err = model.NewAppError("addLicense", "api.license.add_license.open.app_error", nil, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	buf := bytes.NewBuffer(nil)
	io.Copy(buf, file)

	license, appErr := c.App.SaveLicense(buf.Bytes())
	if appErr != nil {
		if appErr.Id == model.EXPIRED_LICENSE_ERROR {
			c.LogAudit("failed - expired or non-started license")
		} else if appErr.Id == model.INVALID_LICENSE_ERROR {
			c.LogAudit("failed - invalid license")
		} else {
			c.LogAudit("failed - unable to save license")
		}
		c.Err = appErr
		return
	}

	if *c.App.Config().JobSettings.RunJobs {
		c.App.Srv().Jobs.Workers = c.App.Srv().Jobs.InitWorkers()
		c.App.Srv().Jobs.StartWorkers()
	}

	auditRec.Success()
	c.LogAudit("success")

	w.Write([]byte(license.ToJson()))
}

func removeLicense(c *Context, w http.ResponseWriter, r *http.Request) {
	auditRec := c.MakeAuditRecord("removeLicense", audit.Fail)
	defer c.LogAuditRec(auditRec)
	c.LogAudit("attempt")

	if err := c.App.RemoveLicense(); err != nil {
		c.Err = err
		return
	}

	auditRec.Success()
	c.LogAudit("success")

	ReturnStatusOK(w)
}
