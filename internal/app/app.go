package app

import (
	"errors"
	"flag"
	"git.haefelfinger.net/piwigo/PiwigoDirectoryUploader/internal/pkg/localFileStructure"
	"git.haefelfinger.net/piwigo/PiwigoDirectoryUploader/internal/pkg/piwigo"
	"github.com/sirupsen/logrus"
	"os"
)

var (
	imagesRootPath            = flag.String("imagesRootPath", "", "This is the images root path that should be mirrored to piwigo.")
	piwigoUrl                 = flag.String("piwigoUrl", "", "The root url without tailing slash to your piwigo installation.")
	piwigoUser                = flag.String("piwigoUser", "", "The username to use during sync.")
	piwigoPassword            = flag.String("piwigoPassword", "", "This is password to the given username.")
	piwigoUploadChunkSizeInKB = flag.Int("piwigoUploadChunkSizeInKB", 512, "The chunksize used to upload an image to piwigo.")
)

func Run() {
	context, err := configureContext()
	if err != nil {
		logErrorAndExit(err, 1)
	}

	err = loginToPiwigoAndConfigureContext(context)
	if err != nil {
		logErrorAndExit(err, 2)
	}

	filesystemNodes, err := localFileStructure.ScanLocalFileStructure(context.LocalRootPath)
	if err != nil {
		logErrorAndExit(err, 3)
	}

	categories, err := getAllCategoriesFromServer(context)
	if err != nil {
		logErrorAndExit(err, 4)
	}

	err = synchronizeCategories(context, filesystemNodes, categories)
	if err != nil {
		logErrorAndExit(err, 5)
	}

	err = synchronizeImages(context, filesystemNodes, categories)
	if err != nil {
		logErrorAndExit(err, 6)
	}

	_ = piwigo.Logout(context.Piwigo)
}

func configureContext() (*appContext, error) {
	logrus.Infoln("Preparing application context and configuration")

	if *piwigoUrl == "" {
		return nil, errors.New("missing piwigo url!")
	}

	if *piwigoUser == "" {
		return nil, errors.New("missing piwigo user!")
	}

	if *piwigoPassword == "" {
		return nil, errors.New("missing piwigo password!")
	}

	context := new(appContext)
	context.LocalRootPath = *imagesRootPath
	context.Piwigo = new(piwigo.PiwigoContext)
	err := context.Piwigo.Initialize(*piwigoUrl, *piwigoUser, *piwigoPassword, *piwigoUploadChunkSizeInKB)

	return context, err
}

func loginToPiwigoAndConfigureContext(context *appContext) error {
	logrus.Infoln("Logging in to piwigo and getting chunk size configuration for uploads")
	err := piwigo.Login(context.Piwigo)
	if err != nil {
		return err
	}
	return initializeUploadChunkSize(context)
}

func initializeUploadChunkSize(context *appContext) error {
	userStatus, err := piwigo.GetStatus(context.Piwigo)
	if err != nil {
		return err
	}
	context.ChunkSizeBytes = userStatus.Result.UploadFormChunkSize * 1024
	logrus.Debugln(context.ChunkSizeBytes)
	return nil
}

func logErrorAndExit(err error, exitCode int) {
	logrus.Errorln(err)
	os.Exit(exitCode)
}
