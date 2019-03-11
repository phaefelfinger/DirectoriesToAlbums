package piwigo

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type uploadChunkResponse struct {
	Status string      `json:"stat"`
	Result interface{} `json:"result"`
}

type fileAddResponse struct {
	Status string `json:"stat"`
	Result struct {
		ImageID int    `json:"image_id"`
		URL     string `json:"url"`
	} `json:"result"`
}

type imageExistResponse struct {
	Stat   string            `json:"stat"`
	Result map[string]string `json:"result"`
}

func ImageUploadRequired(context *PiwigoContext, md5sums []string) ([]string, error) {
	//TODO: make sure to split to multiple queries -> to honor max upload queries
	//TODO: Make sure to return the found imageIds of the found sums to update the local image nodes

	md5sumList := strings.Join(md5sums, ",")

	formData := url.Values{}
	formData.Set("method", "pwg.images.exist")
	formData.Set("md5sum_list", md5sumList)

	logrus.Tracef("Looking up missing files: %s", md5sumList)

	response, err := context.PostForm(formData)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	var imageExistResponse imageExistResponse
	if err := json.NewDecoder(response.Body).Decode(&imageExistResponse); err != nil {
		logrus.Errorln(err)
		return nil, err
	}

	missingFiles := make([]string, 0, len(imageExistResponse.Result))

	for key, value := range imageExistResponse.Result {
		if value == "" {
			logrus.Tracef("Missing file with md5sum: %s", key)
			missingFiles = append(missingFiles, key)
		}
	}

	return missingFiles, nil
}

func UploadImage(context *PiwigoContext, filePath string, md5sum string, category int) (int, error) {
	if context.ChunkSizeInKB <= 0 {
		return 0, errors.New("Uploadchunk size is less or equal to zero. 512 is a recommendet value to begin with.")
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}

	fileSizeInKB := fileInfo.Size() / 1024
	logrus.Infof("Uploading %s using chunksize of %d KB and total size of %d KB", filePath, context.ChunkSizeInKB, fileSizeInKB)

	err = uploadImageChunks(filePath, context, fileSizeInKB, md5sum)
	if err != nil {
		return 0, err
	}

	imageId, err := uploadImageFinal(context, fileInfo.Name(), md5sum, category)
	if err != nil {
		return 0, err
	}

	return imageId, nil
}

func uploadImageChunks(filePath string, context *PiwigoContext, fileSizeInKB int64, md5sum string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	buffer := make([]byte, context.ChunkSizeInKB*1024)
	numberOfChunks := (fileSizeInKB / int64(context.ChunkSizeInKB)) + 1
	currentChunk := int64(0)

	for {
		logrus.Tracef("Processing chunk %d of %d of %s", currentChunk, numberOfChunks, filePath)

		readBytes, readError := reader.Read(buffer)
		if readError == io.EOF && readBytes == 0 {
			break
		}
		if readError != io.EOF && readError != nil {
			return readError
		}

		encodedChunk := base64.StdEncoding.EncodeToString(buffer[:readBytes])

		uploadError := uploadImageChunk(context, encodedChunk, md5sum, currentChunk)
		if uploadError != nil {
			return uploadError
		}

		currentChunk++
	}

	return nil
}

func uploadImageChunk(context *PiwigoContext, base64chunk string, md5sum string, position int64) error {
	formData := url.Values{}
	formData.Set("method", "pwg.images.addChunk")
	formData.Set("data", base64chunk)
	formData.Set("original_sum", md5sum)
	// required by the API for compatibility
	formData.Set("type", "file")
	formData.Set("position", strconv.FormatInt(position, 10))

	logrus.Tracef("Uploading chunk %d of file with sum %s", position, md5sum)

	response, err := context.PostForm(formData)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	var uploadChunkResponse uploadChunkResponse
	if err := json.NewDecoder(response.Body).Decode(&uploadChunkResponse); err != nil {
		logrus.Errorln(err)
		return err
	}

	if uploadChunkResponse.Status != "ok" {
		logrus.Errorf("Got state %s while uploading chunk %d of %s", uploadChunkResponse.Status, position, md5sum)
		return errors.New(fmt.Sprintf("Got state %s while uploading chunk %d of %s", uploadChunkResponse.Status, position, md5sum))
	}

	return nil
}

func uploadImageFinal(context *PiwigoContext, originalFilename string, md5sum string, categoryId int) (int, error) {
	formData := url.Values{}
	formData.Set("method", "pwg.images.add")
	formData.Set("original_sum", md5sum)
	formData.Set("original_filename", originalFilename)
	formData.Set("name", originalFilename)
	formData.Set("categories", strconv.Itoa(categoryId))

	logrus.Debugf("Finalizing upload of file %s with sum %s to category %d", originalFilename, md5sum, categoryId)

	response, err := context.PostForm(formData)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()

	var fileAddResponse fileAddResponse
	if err := json.NewDecoder(response.Body).Decode(&fileAddResponse); err != nil {
		logrus.Errorln(err)
		return 0, err
	}

	if fileAddResponse.Status != "ok" {
		logrus.Errorf("Got state %s while adding image %s", fileAddResponse.Status, originalFilename)
		return 0, errors.New(fmt.Sprintf("Got state %s while adding image %s", fileAddResponse.Status, originalFilename))
	}

	return fileAddResponse.Result.ImageID, nil
}